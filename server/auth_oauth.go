package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// ===========================================================================
// OAuth / Multi-User Auth with Role-Based Access Control (RBAC)
// Supports GitHub OAuth as the primary identity provider.
// ===========================================================================

type UserRole string

const (
	RoleAdmin     UserRole = "admin"
	RoleDeveloper UserRole = "developer"
	RoleViewer    UserRole = "viewer"
)

type UserProfile struct {
	ID        string   `json:"id"`
	Username  string   `json:"username"`
	AvatarURL string   `json:"avatar_url"`
	Role      UserRole `json:"role"`
	CreatedAt string   `json:"created_at"`
}

// TopicPermissions defines which bus topics each role can publish to
var TopicPermissions = map[UserRole][]string{
	RoleAdmin: {"*"}, // full access
	RoleDeveloper: {
		"vm.spawn", "task.run", "editor.typing", "terminal.typing",
		"sys.terminal.intent", "vfs:read", "vfs:semantic",
		"plugin.run", "applet.compile", "pkg.install",
		"sys.config:get", "sys.clipboard:copy", "sys.notify",
		"ai.chat",
	},
	RoleViewer: {
		"vfs:read", "vfs:semantic", "sys.config:get", "ai.chat",
	},
}

// CheckPermission verifies if a role can publish to a given topic
func CheckPermission(role UserRole, topic string) bool {
	allowed, exists := TopicPermissions[role]
	if !exists {
		return false
	}
	for _, t := range allowed {
		if t == "*" || t == topic || strings.HasPrefix(topic, t) {
			return true
		}
	}
	return false
}

// InitOAuthRoutes registers the GitHub OAuth endpoints on the HTTP server
func InitOAuthRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/auth/github", handleGitHubLogin)
	mux.HandleFunc("/auth/github/callback", handleGitHubCallback)
	mux.HandleFunc("/auth/me", handleAuthMe)
}

// handleGitHubLogin redirects to GitHub OAuth authorization
func handleGitHubLogin(w http.ResponseWriter, r *http.Request) {
	clientID := GlobalSysDB.GetConfig("github_client_id")
	if clientID == "" {
		http.Error(w, "GitHub OAuth not configured. Set github_client_id in sys.config", http.StatusServiceUnavailable)
		return
	}

	// Generate state token for CSRF protection
	stateBytes := make([]byte, 16)
	rand.Read(stateBytes)
	state := hex.EncodeToString(stateBytes)
	GlobalSysDB.SetConfig("oauth_state_"+state, "valid")

	redirectURL := "https://github.com/login/oauth/authorize?" +
		"client_id=" + url.QueryEscape(clientID) +
		"&scope=read:user" +
		"&state=" + state

	http.Redirect(w, r, redirectURL, http.StatusTemporaryRedirect)
}

// handleGitHubCallback exchanges the auth code for a token and creates a session
func handleGitHubCallback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")

	// Validate CSRF state
	if GlobalSysDB.GetConfig("oauth_state_"+state) != "valid" {
		http.Error(w, "Invalid state parameter", http.StatusForbidden)
		return
	}
	// Consume the state token
	GlobalSysDB.SetConfig("oauth_state_"+state, "used")

	clientID := GlobalSysDB.GetConfig("github_client_id")
	clientSecret := GlobalSysDB.GetConfig("github_client_secret")

	// Exchange code for access token
	tokenReqBody := url.Values{}
	tokenReqBody.Set("client_id", clientID)
	tokenReqBody.Set("client_secret", clientSecret)
	tokenReqBody.Set("code", code)

	req, _ := http.NewRequest("POST", "https://github.com/login/oauth/access_token", strings.NewReader(tokenReqBody.Encode()))
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		http.Error(w, "Failed to exchange token: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var tokenResp struct {
		AccessToken string `json:"access_token"`
	}
	json.Unmarshal(body, &tokenResp)

	if tokenResp.AccessToken == "" {
		http.Error(w, "GitHub did not return an access token", http.StatusInternalServerError)
		return
	}

	// Fetch user profile from GitHub
	userReq, _ := http.NewRequest("GET", "https://api.github.com/user", nil)
	userReq.Header.Set("Authorization", "Bearer "+tokenResp.AccessToken)
	userResp, err := http.DefaultClient.Do(userReq)
	if err != nil {
		http.Error(w, "Failed to fetch GitHub profile", http.StatusInternalServerError)
		return
	}
	defer userResp.Body.Close()

	var ghUser struct {
		ID        int    `json:"id"`
		Login     string `json:"login"`
		AvatarURL string `json:"avatar_url"`
	}
	userBody, _ := io.ReadAll(userResp.Body)
	json.Unmarshal(userBody, &ghUser)

	// Determine role: first user is admin, rest are developers
	userKey := "user:" + ghUser.Login
	existingRole := GlobalSysDB.GetConfig(userKey + ":role")
	if existingRole == "" {
		// Check if there are any existing users
		firstUser := GlobalSysDB.GetConfig("first_user_registered")
		if firstUser == "" {
			existingRole = string(RoleAdmin)
			GlobalSysDB.SetConfig("first_user_registered", ghUser.Login)
		} else {
			existingRole = string(RoleDeveloper)
		}
		GlobalSysDB.SetConfig(userKey+":role", existingRole)
		GlobalSysDB.SetConfig(userKey+":avatar", ghUser.AvatarURL)
	}

	// Create JWT session with user identity
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":      ghUser.Login,
		"role":     existingRole,
		"avatar":   ghUser.AvatarURL,
		"exp":      time.Now().Add(30 * 24 * time.Hour).Unix(),
	})
	signedToken, err := token.SignedString(JWTSecret)
	if err != nil {
		http.Error(w, "Failed to create session", http.StatusInternalServerError)
		return
	}

	GlobalSysDB.LogAudit("auth.login", ghUser.Login, "GitHub OAuth login successful, role: "+existingRole)

	// Set the session cookie and redirect to the OS desktop
	http.SetCookie(w, &http.Cookie{
		Name:     "kernos_session",
		Value:    signedToken,
		Path:     "/",
		HttpOnly: true,
		MaxAge:   30 * 24 * 60 * 60,
	})

	log.Printf("[OAuth] User '%s' authenticated via GitHub (role: %s)", ghUser.Login, existingRole)
	http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
}

// handleAuthMe returns the current user's profile based on their JWT session
func handleAuthMe(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("kernos_session")
	if err != nil {
		http.Error(w, "Not authenticated", http.StatusUnauthorized)
		return
	}

	token, err := jwt.Parse(cookie.Value, func(t *jwt.Token) (interface{}, error) {
		return JWTSecret, nil
	})
	if err != nil || !token.Valid {
		http.Error(w, "Invalid session", http.StatusUnauthorized)
		return
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		http.Error(w, "Invalid token claims", http.StatusUnauthorized)
		return
	}

	profile := UserProfile{
		Username:  claims["sub"].(string),
		AvatarURL: claims["avatar"].(string),
		Role:      UserRole(claims["role"].(string)),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(profile)
}
