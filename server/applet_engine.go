package main

import (
	"log"

	"github.com/evanw/esbuild/pkg/api"
)

// AppletEngine handles on-the-fly transpilation of React/TypeScript
// written in the Kernos Editor into browser-safe JavaScript.
type AppletEngine struct {
	bus *Bus
}

func NewAppletEngine(bus *Bus) *AppletEngine {
	return &AppletEngine{bus: bus}
}

// CompileApplet takes raw TSX code and uses esbuild to compile it targeting a modern browser.
// The output is bundled as an IIFE (Immediately Invoked Function Expression)
// that returns the React component so the frontend can mount it.
func (ae *AppletEngine) CompileApplet(env Envelope) {
	payload, ok := env.Payload.(map[string]interface{})
	if !ok {
		return
	}

	appletID, _ := payload["appletId"].(string)
	rawTSX, _ := payload["code"].(string)

	if rawTSX == "" {
		ae.bus.Publish(Envelope{
			Topic:   "applet.compile:error",
			From:    "kernel",
			To:      env.From,
			Payload: map[string]string{"error": "Empty source code"},
		})
		return
	}

	log.Printf("[Applet Engine] Compiling dynamic applet: %s", appletID)

	// We compile the TSX into an IIFE so we can execute it securely
	// and extract the default exported React component.
	result := api.Transform(rawTSX, api.TransformOptions{
		Loader:     api.LoaderTSX,
		Format:     api.FormatIIFE,
		Target:     api.ES2020,
		GlobalName: "KernosDynamicApplet",
	})

	if len(result.Errors) > 0 {
		var errStr string
		for _, e := range result.Errors {
			errStr += e.Text + "\n"
		}

		log.Printf("[Applet Engine] Compilation failed: %s", errStr)
		ae.bus.Publish(Envelope{
			Topic:   "applet.compile:error",
			From:    "kernel",
			To:      env.From,
			Payload: map[string]string{"error": errStr},
		})
		return
	}

	// Transform successful
	compiledJS := string(result.Code)

	log.Printf("[Applet Engine] Successfully compiled %s (%.2f kb)", appletID, float64(len(compiledJS))/1024.0)

	ae.bus.Publish(Envelope{
		Topic: "applet.compile:success",
		From:  "kernel",
		To:    env.From,
		Payload: map[string]string{
			"appletId": appletID,
			"code":     compiledJS,
		},
	})
}
