package analyzer

import "testing"

func TestResolveModelPreferred(t *testing.T) {
	got, ok := ResolveModel("google/gemma-4-e4b", []ModelInfo{
		{ID: "qwen2.5-0.5b-instruct-mlx"},
		{ID: "google/gemma-4-e4b"},
		{ID: "text-embedding-nomic-embed-text-v1.5"},
	})
	if !ok || got != "google/gemma-4-e4b" {
		t.Fatalf("got %q ok=%v", got, ok)
	}
}

func TestResolveModelGemmaFallback(t *testing.T) {
	got, ok := ResolveModel("missing", []ModelInfo{
		{ID: "google/gemma-4-12b"},
		{ID: "text-embedding-nomic-embed-text-v1.5"},
	})
	if !ok || got != "google/gemma-4-12b" {
		t.Fatalf("got %q ok=%v", got, ok)
	}
}

func TestResolveModelPreferE4BAmongGemma(t *testing.T) {
	got, ok := ResolveModel("missing", []ModelInfo{
		{ID: "google/gemma-4-12b"},
		{ID: "google/gemma-4-e4b"},
	})
	if !ok || got != "google/gemma-4-e4b" {
		t.Fatalf("got %q ok=%v", got, ok)
	}
}

func TestResolveModelFirstChatSkipsEmbedding(t *testing.T) {
	got, ok := ResolveModel("missing", []ModelInfo{
		{ID: "text-embedding-nomic-embed-text-v1.5"},
		{ID: "qwen2.5-0.5b-instruct-mlx"},
	})
	if !ok || got != "qwen2.5-0.5b-instruct-mlx" {
		t.Fatalf("got %q ok=%v", got, ok)
	}
}

func TestResolveModelEmpty(t *testing.T) {
	_, ok := ResolveModel("google/gemma-4-e4b", nil)
	if ok {
		t.Fatal("expected not ok")
	}
}
