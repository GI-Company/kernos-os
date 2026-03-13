package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// The Vector Engine handles semantic embeddings and nearest-neighbor search.
// This forms the underlying fabric of the Kernos OS Cognitive Microkernel,
// powering both the Neural Routing bus and the Holographic VFS.

type DocumentChunk struct {
	ID             string    `json:"id"`
	FilePath       string    `json:"file_path"`
	StartLine      int       `json:"start_line"`
	EndLine        int       `json:"end_line"`
	Text           string    `json:"text"`
	Vector         []float32 `json:"vector,omitempty"`
	Weight         float32   `json:"weight"`           // Phase 14: Synaptic Weight
	LastAccessTime time.Time `json:"last_access_time"` // Phase 14: Temporal Decay
}

type VectorDB struct {
	mu     sync.RWMutex
	Chunks []DocumentChunk

	// Nomic Embeddings API configuration
	APIURL string
	Model  string

	// SQLite Persistence
	DB *sql.DB

	// Phase 14: Unconventional Trinity (Sensory Cortex Hot Cache)
	HotCache      []SearchResult
	HotCacheMutex sync.RWMutex
}

// Global Vector Database instance
var GlobalVectorDB *VectorDB

func InitVectorDB(lmStudioURL string) *VectorDB {
	// E.g. "http://127.0.0.1:1234/v1/chat/completions" -> "http://127.0.0.1:1234/v1/embeddings"
	embedURL := strings.Replace(lmStudioURL, "/chat/completions", "/embeddings", 1)

	dbPath := ".kernos/vfs.db"
	os.MkdirAll(".kernos", 0755)
	
	sqliteDB, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		log.Fatalf("[VectorDB] Failed to open SQLite DB: %v", err)
	}

	schemaQueries := []string{
		`CREATE TABLE IF NOT EXISTS chunks (
			id TEXT PRIMARY KEY,
			file_path TEXT,
			start_line INTEGER,
			end_line INTEGER,
			text TEXT,
			vector BLOB,
			weight REAL,
			last_access_time DATETIME
		);`,
		`CREATE TABLE IF NOT EXISTS entities (
			id TEXT PRIMARY KEY,
			name TEXT,
			type TEXT,
			description TEXT
		);`,
		`CREATE TABLE IF NOT EXISTS relationships (
			source TEXT,
			target TEXT,
			type TEXT,
			description TEXT,
			PRIMARY KEY (source, target, type)
		);`,
		`CREATE TABLE IF NOT EXISTS chunk_entities (
			chunk_id TEXT,
			entity_id TEXT,
			PRIMARY KEY (chunk_id, entity_id)
		);`,
	}
	
	for _, q := range schemaQueries {
		if _, err := sqliteDB.Exec(q); err != nil {
			log.Fatalf("[VectorDB] Failed to create schema: %v", err)
		}
	}

	db := &VectorDB{
		Chunks:   make([]DocumentChunk, 0),
		APIURL:   embedURL,
		Model:    "text-embedding-nomic-embed-text-v1.5",
		DB:       sqliteDB,
		HotCache: make([]SearchResult, 0),
	}
	db.LoadFromDB()
	
	// Start the Background Graph Builder for GraphRAG
	go db.startGraphBuilderWorker(lmStudioURL)

	GlobalVectorDB = db
	return db
}

func (db *VectorDB) LoadFromDB() {
	rows, err := db.DB.Query("SELECT id, file_path, start_line, end_line, text, vector, weight, last_access_time FROM chunks")
	if err != nil {
		log.Printf("[VectorDB] Error loading chunks from DB: %v", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var chunk DocumentChunk
		var vectorBytes []byte
		var lastAccessTime string

		err := rows.Scan(&chunk.ID, &chunk.FilePath, &chunk.StartLine, &chunk.EndLine, &chunk.Text, &vectorBytes, &chunk.Weight, &lastAccessTime)
		if err != nil {
			continue
		}

		if len(vectorBytes) > 0 {
			json.Unmarshal(vectorBytes, &chunk.Vector)
		}
		chunk.LastAccessTime, _ = time.Parse(time.RFC3339, lastAccessTime)

		db.Chunks = append(db.Chunks, chunk)
	}
	log.Printf("[VectorDB] Successfully loaded %d chunks from SQLite disk.", len(db.Chunks))
}

func (db *VectorDB) SaveChunkToDB(chunk DocumentChunk) {
	vectorBytes, _ := json.Marshal(chunk.Vector)
	query := `
		INSERT INTO chunks (id, file_path, start_line, end_line, text, vector, weight, last_access_time)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			vector=excluded.vector,
			weight=excluded.weight,
			last_access_time=excluded.last_access_time;
	`
	_, err := db.DB.Exec(query, chunk.ID, chunk.FilePath, chunk.StartLine, chunk.EndLine, chunk.Text, vectorBytes, chunk.Weight, chunk.LastAccessTime.Format(time.RFC3339))
	if err != nil {
		log.Printf("[VectorDB] Error saving chunk %s: %v", chunk.ID, err)
	}
}

// ---------------------------------------------------------------------------
// Embedding Generation
// ---------------------------------------------------------------------------

type embedRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type embedResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
}

// GenerateEmbeddings calls the local LM Studio Nomic model to vectorize text.
func (db *VectorDB) GenerateEmbeddings(texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	reqBody := embedRequest{
		Model: db.Model,
		Input: texts,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	resp, err := http.Post(db.APIURL, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("embedding API unreachable at %s: %w", db.APIURL, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var embResp embedResponse
	if err := json.Unmarshal(body, &embResp); err != nil {
		return nil, fmt.Errorf("failed to parse embedding response: %w", err)
	}

	if len(embResp.Data) != len(texts) {
		return nil, fmt.Errorf("expected %d embeddings, got %d", len(texts), len(embResp.Data))
	}

	vectors := make([][]float32, len(texts))
	for i, d := range embResp.Data {
		vectors[i] = d.Embedding
	}

	return vectors, nil
}

// ---------------------------------------------------------------------------
// Vector Math
// ---------------------------------------------------------------------------

func CosineSimilarity(a, b []float32) float32 {
	if len(a) != len(b) || len(a) == 0 {
		return 0.0
	}
	var dotProduct, normA, normB float64
	for i := range a {
		valA := float64(a[i])
		valB := float64(b[i])
		dotProduct += valA * valB
		normA += valA * valA
		normB += valB * valB
	}
	if normA == 0 || normB == 0 {
		return 0.0
	}
	return float32(dotProduct / (math.Sqrt(normA) * math.Sqrt(normB)))
}

type SearchResult struct {
	Chunk      DocumentChunk
	Similarity float32
}

// Search returns the top K most semantically similar chunks to the query vector.
// Phase 14: Acts as the Hippocampus trigger. RAG retrieval permanently restructures the OS graph.
func (db *VectorDB) Search(queryVector []float32, topK int) []SearchResult {
	db.mu.RLock()

	var results []SearchResult
	for _, chunk := range db.Chunks {
		if len(chunk.Vector) == 0 {
			continue
		}
		sim := CosineSimilarity(queryVector, chunk.Vector)

		// Phase 14: Mutate similarity by Synaptic Weight
		weightedSim := sim * chunk.Weight

		results = append(results, SearchResult{Chunk: chunk, Similarity: weightedSim})
	}
	db.mu.RUnlock()

	// Sort
	for i := 0; i < len(results); i++ {
		for j := i + 1; j < len(results); j++ {
			if results[j].Similarity > results[i].Similarity {
				results[i], results[j] = results[j], results[i]
			}
		}
	}

	if len(results) > topK {
		results = results[:topK]
	}

	// Phase 14: Hebbian Learning Interaction Boost (phi)
	db.mu.Lock()
	defer db.mu.Unlock()
	for i, res := range results {
		for j, chunk := range db.Chunks {
			if chunk.ID == res.Chunk.ID {
				db.Chunks[j].Weight += 0.05
				db.Chunks[j].LastAccessTime = time.Now()
				results[i].Chunk = db.Chunks[j]
				break
			}
		}
	}

	return results
}

// CompileGraphContext queries the Vector search space for top K semantics, 
// then executes an SQL Graph Traversal to extract relational edges from the GraphRAG entities table.
// Phase 14: Integrating GraphRAG directly into the Architect context wave.
func (db *VectorDB) CompileGraphContext(query string, topK int) string {
	// First, get the raw semantic neighborhood
	vectors, err := db.GenerateEmbeddings([]string{query})
	if err != nil || len(vectors) == 0 {
		return ""
	}

	results := db.Search(vectors[0], topK)
	if len(results) == 0 {
		return ""
	}

	var contextBuilder strings.Builder
	contextBuilder.WriteString("🕸️ GraphRAG Context (Knowledge Graph & Semantic Sources):\n\n")

	for _, res := range results {
		// 1. Append raw source context
		contextBuilder.WriteString(fmt.Sprintf("-- SOURCE [%s] (Sim: %.2f) --\n%s\n\n", res.Chunk.FilePath, res.Similarity, res.Chunk.Text))

		// 2. Perform SQLite Graph Traversal on the local context node
		rows, err := db.DB.Query(`
			SELECT e.name, e.type, e.description
			FROM entities e
			JOIN chunk_entities ce ON e.id = ce.entity_id
			WHERE ce.chunk_id = ?
		`, res.Chunk.ID)

		if err == nil {
			var entities []string
			for rows.Next() {
				var name, etype, desc string
				if err := rows.Scan(&name, &etype, &desc); err == nil {
					entities = append(entities, fmt.Sprintf("- [%s] %s: %s", etype, name, desc))
				}
			}
			rows.Close()

			if len(entities) > 0 {
				contextBuilder.WriteString("🔗 Graph Entities Extracted:\n")
				contextBuilder.WriteString(strings.Join(entities, "\n"))
				contextBuilder.WriteString("\n\n")
			}
		}
	}

	return contextBuilder.String()
}

// DecayWeights implements the Hippocampus temporal decay equation: W_new = W_old * e^(-lambda * dt)
func (db *VectorDB) DecayWeights() {
	db.mu.Lock()
	defer db.mu.Unlock()

	lambda := 0.05 // Decay rate per hour
	now := time.Now()

	for i, chunk := range db.Chunks {
		if chunk.Weight == 0 {
			db.Chunks[i].Weight = 1.0 // Initialize if 0
			db.Chunks[i].LastAccessTime = now
			continue
		}

		dtHours := now.Sub(chunk.LastAccessTime).Hours()
		if dtHours < 0 {
			dtHours = 0
		}

		// Decay equation
		newWeight := chunk.Weight * float32(math.Exp(-lambda*dtHours))

		// Floor it at 0.1 so we don't permanently lose items in latent space
		if newWeight < 0.1 {
			newWeight = 0.1
		}

		db.Chunks[i].Weight = newWeight
		
		// Optionally flush weight back to DB (Commented out to save excessive disk I/O, rely on memory till exit/re-index)
		// db.SaveChunkToDB(db.Chunks[i])
	}
	log.Printf("[Hippocampus] Synaptic plasticity decay applied to %d memory nodes.", len(db.Chunks))
}

// ---------------------------------------------------------------------------
// Document Indexing
// ---------------------------------------------------------------------------

// AutoIndexWorkspace scans the given directory asynchronously, chunking source files
// and generating embeddings in the background without blocking the main OS thread.
func (db *VectorDB) AutoIndexWorkspace(dirPath string) {
	log.Printf("[VectorDB] ⏳ Beginning asynchronous background ingestion of: %s", dirPath)

	go func() {
		startTime := time.Now()
		var allChunks []DocumentChunk
		var textsToEmbed []string

		err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil
			}

			// STRICT SCALABILITY RULES: Ignore massive, binary, or irrelevant folders
			if strings.Contains(path, "/.") ||
				strings.Contains(path, ".kernos/") ||
				strings.Contains(path, "/dist/") ||
				strings.Contains(path, "node_modules") ||
				strings.Contains(path, "/vendor/") ||
				strings.Contains(path, "/build/") {
				return nil
			}
			ext := filepath.Ext(path)
			if ext == "" || ext == ".png" || ext == ".jpg" || ext == ".exe" || ext == ".zip" || ext == ".tar" || ext == ".db" || ext == ".sqlite" {
				return nil // Source files only
			}

			content, err := os.ReadFile(path)
			if err != nil {
				return nil
			}

			// Very basic chunking strategy: split by blank lines or rough byte count.
			lines := strings.Split(string(content), "\n")
			chunkSize := 30

			for i := 0; i < len(lines); i += chunkSize {
				end := i + chunkSize
				if end > len(lines) {
					end = len(lines)
				}
				chunkText := strings.Join(lines[i:end], "\n")

				// Skip tiny chunks
				if len(strings.TrimSpace(chunkText)) < 20 {
					continue
				}

				relPath, _ := filepath.Rel(dirPath, path)
				if relPath == "" {
					relPath = path
				}

				chunk := DocumentChunk{
					ID:             fmt.Sprintf("%s:%d-%d", relPath, i+1, end),
					FilePath:       relPath,
					StartLine:      i + 1,
					EndLine:        end,
					Text:           chunkText,
					Weight:         1.0,        // Phase 14: Base synapse weight
					LastAccessTime: time.Now(), // Phase 14
				}
				allChunks = append(allChunks, chunk)

				// Nomic embed prefix for clustering
				textsToEmbed = append(textsToEmbed, "search_document: "+chunkText)
			}
			return nil
		})

		if err != nil {
			log.Printf("[VectorDB] Walk error: %v", err)
		}

		log.Printf("[VectorDB] Found %d valid source chunks. Embedding in background paginated batches...", len(allChunks))

		// Batch queries slowly to avoid LM Studio OOM crashes
		// (Simulating a true distributed ingestion queue)
		batchSize := 10
		for i := 0; i < len(textsToEmbed); i += batchSize {
			end := i + batchSize
			if end > len(textsToEmbed) {
				end = len(textsToEmbed)
			}

			vectors, err := db.GenerateEmbeddings(textsToEmbed[i:end])
			if err != nil {
				log.Printf("[VectorDB] Embedding error at batch %d: %v", i, err)
				time.Sleep(2 * time.Second) // Backoff
				continue
			}

			db.mu.Lock()
			for j, vec := range vectors {
				chunk := allChunks[i+j]
				chunk.Vector = vec
				
				// Persist vector to SQLite
				db.SaveChunkToDB(chunk)
				
				// Add to memory
				db.Chunks = append(db.Chunks, chunk)
			}
			db.mu.Unlock()

			// Yield execution slightly to ensure OS UI/Message Bus remains responsive
			time.Sleep(100 * time.Millisecond)
		}

		log.Printf("[VectorDB] ✅ Background Semantic Indexing Complete: %d chunks ready in %v", len(db.Chunks), time.Since(startTime))
	}()
}
