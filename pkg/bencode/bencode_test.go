package bencode

import (
	"testing"
)

func TestBencodeDecodeString(t *testing.T) {
	data := []byte("4:spam")
	result, err := Decode(data)
	if err != nil {
		t.Fatal(err)
	}

	str, ok := result.(string)
	if !ok {
		t.Fatalf("expected string, got %T", result)
	}

	if str != "spam" {
		t.Errorf("expected 'spam', got '%s'", str)
	}
}

func TestBencodeDecodeInt(t *testing.T) {
	data := []byte("i42e")
	result, err := Decode(data)
	if err != nil {
		t.Fatal(err)
	}

	i, ok := result.(int)
	if !ok {
		t.Fatalf("expected int, got %T", result)
	}

	if i != 42 {
		t.Errorf("expected 42, got %d", i)
	}
}

func TestBencodeDecodeDict(t *testing.T) {
	data := []byte("d3:cow3:moo4:spam4:eggse")
	result, err := Decode(data)
	if err != nil {
		t.Fatal(err)
	}

	dict, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map[string]interface{}, got %T", result)
	}

	if len(dict) != 2 {
		t.Fatalf("expected dict length 2, got %d", len(dict))
	}

	if val, ok := dict["cow"]; !ok || val != "moo" {
		t.Errorf(`expected "cow" = "moo", got %v (type %T)`, val, val)
	}

	if val, ok := dict["spam"]; !ok || val != "eggs" {
		t.Errorf(`expected "spam" = "eggs", got %v (type %T)`, val, val)
	}
}
