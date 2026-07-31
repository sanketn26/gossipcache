package rpc_test

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"

	"github.com/sanketn26/gossipcache/internal/rpc"
)

func TestWriteGoldenVectors(t *testing.T) {
	if os.Getenv("WRITE_RPC_VECTORS") != "1" {
		t.Skip("set WRITE_RPC_VECTORS=1 to regenerate testdata/rpc_vectors.json")
	}
	messages := goldenMessages()
	vectors := make(map[string]string, len(messages))
	for name, entry := range messages {
		frame, err := rpc.MarshalFrame(entry.CorrelationID, entry.Message)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		vectors[name] = hex.EncodeToString(frame)
	}
	data, err := json.MarshalIndent(vectors, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	data = append(data, '\n')
	if err := os.WriteFile("testdata/rpc_vectors.json", data, 0o644); err != nil {
		t.Fatal(err)
	}
}
