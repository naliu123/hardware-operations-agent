package sandbox

import (
	"testing"
	"time"

	"hwops/internal/domain"
)

func validRequest() Request {
	data := []byte("input")
	return Request{
		ID: "EXECUTIONIDENTITY1234", Code: "print('ok')",
		Image:  "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Budget: domain.PythonLimits(), Deadline: time.Now().Add(time.Minute),
		Inputs: []Input{{ExecutionInput: domain.ExecutionInput{
			ID: "ATTACHMENTIDENTITY1", Kind: "ATTACHMENT", Name: "input.log",
			SHA256: Hash(data), Bytes: int64(len(data)),
		}, Data: data}},
	}
}

func TestRequestValidationAndStableHash(t *testing.T) {
	request := validRequest()
	if err := request.Validate(); err != nil {
		t.Fatal(err)
	}
	hash := request.Hash()
	request.Inputs[0].Data = append([]byte{}, request.Inputs[0].Data...)
	if request.Hash() != hash {
		t.Fatal("request hash depends on transferred bytes instead of their manifest hash")
	}
	tests := []func(*Request){
		func(r *Request) { r.Budget.MemoryBytes++ },
		func(r *Request) { r.Image = "latest" },
		func(r *Request) { r.Inputs[0].Data[0]++ },
		func(r *Request) { r.Inputs[0].Name = "bad\nname" },
		func(r *Request) { r.Inputs = append(r.Inputs, r.Inputs[0]) },
	}
	for i, mutate := range tests {
		invalid := validRequest()
		mutate(&invalid)
		if invalid.Validate() == nil {
			t.Fatalf("invalid request %d accepted", i)
		}
	}
}

func TestClientRejectsRemoteCleartextAndWeakToken(t *testing.T) {
	if _, err := NewClient("http://runner.internal:8091", "01234567890123456789012345678901"); err == nil {
		t.Fatal("remote cleartext runner accepted")
	}
	if _, err := NewClient("https://runner.internal:8091", "short"); err == nil {
		t.Fatal("weak control token accepted")
	}
	if _, err := NewClient("http://127.0.0.1:8091", "01234567890123456789012345678901"); err != nil {
		t.Fatal(err)
	}
}
