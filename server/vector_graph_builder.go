package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

// GraphEntities represents the JSON output from Qwen3.5-9b for GraphRAG
type GraphEntities struct {
	Entities []struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		Type        string `json:"type"`
		Description string `json:"description"`
	} `json:"entities"`
	Relationships []struct {
		Source      string `json:"source"`
		Target      string `json:"target"`
		Type        string `json:"type"`
		Description string `json:"description"`
	} `json:"relationships"`
}

func (db *VectorDB) startGraphBuilderWorker(lmURL string) {
	log.Println("[GraphBuilder] 🕸️ Starting asynchronous Knowledge Graph extractor...")
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		log.Println("[GraphBuilder] ⏰ Ticker fired. Querying SQLite for unprocessed chunks...")
		// 1. Find a chunk that hasn't been processed yet for entities
		query := `
			SELECT c.id, c.text 
			FROM chunks c
			LEFT JOIN chunk_entities ce ON c.id = ce.chunk_id
			WHERE ce.chunk_id IS NULL AND c.text != '' AND c.text IS NOT NULL
			LIMIT 1
		`
		var chunkID, chunkText string
		err := db.DB.QueryRow(query).Scan(&chunkID, &chunkText)
		log.Printf("[GraphBuilder] Query returned. Error status: %v, ChunkID found: %s", err, chunkID)
		if err != nil {
			if err.Error() != "sql: no rows in result set" {
				log.Printf("[GraphBuilder] 🚨 DB Query Error: %v", err)
			} else {
				log.Printf("[GraphBuilder] 💤 No unprocessed chunks found (sql: no rows). Sleeping...")
			}
			continue
		}

		log.Printf("[GraphBuilder] 🧠 Extracting entities from chunk %s...", chunkID)
		
		prompt := `You are a structural code analyzer. Extract entities and their relationships from the provided code chunk.
Return ONLY a valid JSON object with the following structure, and nothing else (no markdown blocks or thinking):
{
  "entities": [ {"id": "unique_id", "name": "Name", "type": "function|struct|agent|module|other", "description": "short description"} ],
  "relationships": [ {"source": "id1", "target": "id2", "type": "calls|uses|implements|related", "description": "context"} ]
}

CODE CHUNK:
` + chunkText

		// 3. Make HTTP call to LM Studio
		extractedJSON, err := queryGraphModel(lmURL, prompt)
		if err != nil {
			log.Printf("[GraphBuilder] ❌ Extraction failed: %v", err)
			continue
		}

		// 4. Parse the JSON
		var graph GraphEntities
		cleanJSON := stripThinkTags(extractedJSON)
		cleanJSON = strings.TrimPrefix(cleanJSON, "```json")
		cleanJSON = strings.TrimPrefix(cleanJSON, "```")
		cleanJSON = strings.TrimSuffix(cleanJSON, "```")
		cleanJSON = strings.TrimSpace(cleanJSON)

		err = json.Unmarshal([]byte(cleanJSON), &graph)
		if err != nil {
			log.Printf("[GraphBuilder] ❌ Failed to parse JSON from model: %v\nRaw output: %s", err, cleanJSON)
			// Mark it as processed anyway so we don't infinitely loop on bad chunks
			db.DB.Exec("INSERT INTO chunk_entities (chunk_id, entity_id) VALUES (?, ?)", chunkID, "PARSE_ERROR")
			continue
		}

		// 5. Insert into Database
		tx, err := db.DB.Begin()
		if err != nil {
			log.Printf("[GraphBuilder] DB error: %v", err)
			continue
		}

		for _, e := range graph.Entities {
			_, err = tx.Exec(`
				INSERT INTO entities (id, name, type, description) 
				VALUES (?, ?, ?, ?)
				ON CONFLICT(id) DO UPDATE SET description=excluded.description
			`, e.ID, e.Name, e.Type, e.Description)
			
			// Link chunk to entity
			tx.Exec(`
				INSERT OR IGNORE INTO chunk_entities (chunk_id, entity_id) 
				VALUES (?, ?)
			`, chunkID, e.ID)
		}

		for _, r := range graph.Relationships {
			_, err = tx.Exec(`
				INSERT INTO relationships (source, target, type, description) 
				VALUES (?, ?, ?, ?)
				ON CONFLICT(source, target, type) DO UPDATE SET description=excluded.description
			`, r.Source, r.Target, r.Type, r.Description)
		}

		// If no entities were found, we still need to record that we processed this chunk
		if len(graph.Entities) == 0 {
			tx.Exec("INSERT INTO chunk_entities (chunk_id, entity_id) VALUES (?, ?)", chunkID, "NO_ENTITIES")
		}

		err = tx.Commit()
		if err != nil {
			log.Printf("[GraphBuilder] ❌ Failed to commit graph data: %v", err)
		} else {
			log.Printf("[GraphBuilder] ✅ Added %d entities and %d relationships for chunk %s", len(graph.Entities), len(graph.Relationships), chunkID)
		}
	}
}

func queryGraphModel(lmURL, prompt string) (string, error) {
	reqBody := map[string]interface{}{
		"model": "qwen/qwen3.5-9b",
		"messages": []map[string]string{
			{"role": "system", "content": "You are a JSON-only structural code analyzer."},
			{"role": "user", "content": prompt},
		},
		"temperature": 0.1, // Low temp for deterministic extraction
	}
	
	jsonData, _ := json.Marshal(reqBody)
	log.Printf("[GraphBuilder] 🌐 Sending HTTP POST to %s", lmURL)
	
	client := &http.Client{
		Timeout: 5 * time.Minute, // Give local 9B model 5 mins to extract entities
	}
	
	req, err := http.NewRequest("POST", lmURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		log.Printf("[GraphBuilder] ❌ HTTP Client error: %v", err)
		return "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	
	var llmResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	
	if err := json.Unmarshal(body, &llmResp); err != nil {
		return "", fmt.Errorf("bad llm response: %w\nBody: %s", err, string(body))
	}

	if len(llmResp.Choices) > 0 {
		return llmResp.Choices[0].Message.Content, nil
	}
	
	return "", fmt.Errorf("no content returned. Raw response: %s", string(body))
}
