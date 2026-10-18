package model

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Embed uses the explicit embedding model, independently of the chat catalog.
func Embed(ctx context.Context, c Config, key, embeddingModel string, texts []string) ([][]float64, int64, error) {
	if err := c.Validate(); err != nil {
		return nil, 0, err
	}
	if c.Provider != "openai" || strings.TrimSpace(embeddingModel) == "" {
		return nil, 0, errors.New("configure an OpenAI embedding connection and model")
	}
	body, err := json.Marshal(map[string]any{"model": embeddingModel, "input": texts, "encoding_format": "float"})
	if err != nil {
		return nil, 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL()+"/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, 0, errors.New("invalid embedding request")
	}
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	client := &http.Client{Timeout: 60 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("embedding redirects are not permitted") }}
	res, err := client.Do(req)
	if err != nil {
		return nil, 0, errors.New("embedding request failed")
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, 0, fmt.Errorf("embedding request returned HTTP %d", res.StatusCode)
	}
	var result struct {
		Data []struct {
			Index     int
			Embedding []float64
		}
		Usage struct {
			Tokens int64 `json:"total_tokens"`
		}
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, 8<<20+1))
	if err != nil || len(data) > 8<<20 {
		return nil, 0, errors.New("embedding response exceeds allowance")
	}
	if err = json.Unmarshal(data, &result); err != nil || len(result.Data) != len(texts) {
		return nil, 0, errors.New("invalid embedding response")
	}
	vectors := make([][]float64, len(texts))
	for _, item := range result.Data {
		if item.Index < 0 || item.Index >= len(vectors) || vectors[item.Index] != nil {
			return nil, 0, errors.New("invalid embedding response indexes")
		}
		vectors[item.Index] = item.Embedding
	}
	return vectors, result.Usage.Tokens, nil
}
