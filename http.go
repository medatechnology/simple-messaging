package simplemessage

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// doJSON performs a JSON request and decodes the success response into out.
// Non-2xx responses are returned as an error with the provider message.
func doJSON(ctx context.Context, httpClient *http.Client, method, url string, body interface{}, out interface{}, errOut interface{}, setAuth func(*http.Request)) error {
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			return fmt.Errorf("simplemessage: encode request: %w", err)
		}
	}

	req, err := http.NewRequestWithContext(ctx, method, url, &buf)
	if err != nil {
		return fmt.Errorf("simplemessage: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if setAuth != nil {
		setAuth(req)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("simplemessage: request %s %s: %w", method, url, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("simplemessage: read response: %w", err)
	}

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		if out != nil && len(respBody) > 0 {
			if err := json.Unmarshal(respBody, out); err != nil {
				return fmt.Errorf("simplemessage: decode response: %w", err)
			}
		}
		return nil
	}
	return fmt.Errorf("simplemessage: provider error (status %d): %s", resp.StatusCode, string(respBody))
}
