package main

import (
	"strings"
	"testing"
)

func TestCosineSimilarity(t *testing.T) {
	// Orthogonal vectors
	a := []float32{1.0, 0.0}
	b := []float32{0.0, 1.0}
	sim := CosineSimilarity(a, b)
	if sim != 0 {
		t.Errorf("Expected 0 similarity for orthogonal vectors, got %f", sim)
	}

	// Identical vectors
	c := []float32{1.0, 2.0, 3.0}
	d := []float32{1.0, 2.0, 3.0}
	sim = CosineSimilarity(c, d)
	// Float comparison with epsilon
	if sim < 0.999 || sim > 1.001 {
		t.Errorf("Expected 1.0 similarity for identical vectors, got %f", sim)
	}

	// Opposite vectors
	e := []float32{1.0, 1.0}
	f := []float32{-1.0, -1.0}
	sim = CosineSimilarity(e, f)
	if sim > -0.999 || sim < -1.001 {
		t.Errorf("Expected -1.0 similarity for opposite vectors, got %f", sim)
	}
}

func TestVectorDBSearch(t *testing.T) {
	db := &VectorDB{
		Chunks: []DocumentChunk{
			{ID: "1", Text: "Apple", Vector: []float32{1.0, 0.0, 0.0}},
			{ID: "2", Text: "Banana", Vector: []float32{0.0, 1.0, 0.0}},
			{ID: "3", Text: "Orange", Vector: []float32{0.0, 0.0, 1.0}},
		},
	}

	// Search for something close to Apple
	query := []float32{0.9, 0.1, 0.0}
	results := db.Search(query, 2)

	if len(results) != 2 {
		t.Fatalf("Expected 2 results, got %d", len(results))
	}

	if results[0].Chunk.ID != "1" {
		t.Errorf("Expected top result to be Apple (ID 1), got %s", results[0].Chunk.ID)
	}

	if results[1].Chunk.ID != "2" {
		t.Errorf("Expected second result to be Banana (ID 2), got %s", results[1].Chunk.ID)
	}
}

func TestSafeImportList(t *testing.T) {
	list := safeImportList()
	if !strings.Contains(list, "fmt") {
		t.Errorf("Expected safeImportList to contain 'fmt', got %s", list)
	}
}
