package agent

import (
	"context"
	"testing"
)

func TestGoogleBackendRegistered(t *testing.T) {
	if !IsSupportedType("google") {
		t.Fatal("google should be in SupportedTypes")
	}

	b, err := New("google", Config{})
	if err != nil {
		t.Fatalf("New(\"google\") error: %v", err)
	}
	if b == nil {
		t.Fatal("New(\"google\") returned nil backend")
	}
}

func TestGoogleListModels(t *testing.T) {
	cat, err := ListModels(context.Background(), "google", Command{})
	if err != nil {
		t.Fatalf("ListModels google error: %v", err)
	}
	if len(cat.Models) == 0 {
		t.Fatal("ListModels google should return models")
	}
	for _, m := range cat.Models {
		t.Logf("Model: %s (%s)", m.ID, m.Label)
	}
}

func TestGoogleExecuteHello(t *testing.T) {
	b, err := New("google", Config{})
	if err != nil {
		t.Fatal(err)
	}
	prompt := "You are running as a local coding agent for a AI Gauntlet workspace.\n\nYour assigned issue ID is: 01a08204-aadf-7599-af9c-47ecd6675aa3\nIssue Title: Hello hello\n\nIssue Description:\nhello how are you?\n\nPlease answer the user's greeting and complete your run.\n"
	sess, err := b.Execute(context.Background(), prompt, ExecOptions{
		Model: "gemini-3.5-flash-lite",
		SystemPrompt: "You are a helpful coding assistant. When tasked with a greeting or simple question, reply nicely and concisely.",
	})
	if err != nil {
		t.Fatal(err)
	}
	for msg := range sess.Messages {
		t.Logf("MSG [%s]: content=%q tool=%s input=%v output=%q", msg.Type, msg.Content, msg.Tool, msg.Input, msg.Output)
	}
	res := <-sess.Result
	if res.Status != "completed" {
		t.Fatalf("expected completed status, got %s: %s", res.Status, res.Error)
	}
	if res.Output == "" {
		t.Fatal("expected non-empty output")
	}
	t.Logf("RESULT: Status=%s Output=%q", res.Status, res.Output)
}

