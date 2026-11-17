package routing

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/gobwas/ws"
)

func TestSelectAndBorrowedKey(t *testing.T) {
	for _, name := range []string{"", "json"} {
		if _, err := Select(name, nil); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := Select("missing", nil); err == nil {
		t.Fatal("unknown plugin accepted")
	}
	wantErr := errors.New("invalid packet")
	plugin := ExtractorFunc(func(op ws.OpCode, payload []byte) ([]byte, error) {
		if op != ws.OpBinary || len(payload) < 2 {
			return nil, wantErr
		}
		return payload[:2], nil
	})
	selected, err := Select("binary", map[string]Extractor{"binary": plugin})
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte("id-body")
	key, err := selected.Extract(ws.OpBinary, payload)
	if err != nil || string(key) != "id" || &key[0] != &payload[0] || string(payload) != "id-body" {
		t.Fatalf("key %q: %v", key, err)
	}
	if _, err := selected.Extract(ws.OpText, payload); !errors.Is(err, wantErr) {
		t.Fatal(err)
	}
}

func FuzzJSONCompatibility(f *testing.F) {
	for _, s := range []string{`{"recipientId":"x"}`, `{}`, `null`, `{"recipientId":null}`, `{"recipientId":1}`, `{"recipientId":1,"recipientId":"last"}`, `{"recipientId":"a\u0062"}`, `{"RecipientId":"x"}`, `{"recipientId":""}`} {
		f.Add([]byte(s))
	}
	originalType := reflect.StructOf([]reflect.StructField{{Name: "RecipientId", Type: reflect.TypeOf(json.RawMessage{}), Tag: `json:"recipientId"`}})
	f.Fuzz(func(t *testing.T, payload []byte) {
		original := reflect.New(originalType).Interface()
		oldErr := json.Unmarshal(payload, original)
		var oldKey string
		if oldErr == nil {
			oldErr = json.Unmarshal(reflect.ValueOf(original).Elem().FieldByName("RecipientId").Interface().(json.RawMessage), &oldKey)
		}
		got, err := (JSON{}).Extract(ws.OpText, payload)
		if (oldErr == nil) != (err == nil) || (err == nil && !bytes.Equal(got, []byte(oldKey))) {
			t.Fatalf("old %q/%v, new %q/%v", oldKey, oldErr, got, err)
		}
	})
}

func BenchmarkBorrowedExtractor(b *testing.B) {
	payload := []byte("id-body")
	plugin, _ := Select("binary", map[string]Extractor{"binary": ExtractorFunc(func(_ ws.OpCode, p []byte) ([]byte, error) { return p[:2], nil })})
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if key, err := plugin.Extract(ws.OpBinary, payload); err != nil || len(key) != 2 {
			b.Fatal(key, err)
		}
	}
}
