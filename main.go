package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
)

// roundTripperFunc is an adapter to allow the use of
// ordinary functions as http.RoundTripper.
type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func main() {
	target := "http://localhost:8080" // Upstream server
	upstream, err := url.Parse(target)
	if err != nil {
		log.Fatalf("Invalid upstream URL: %v", err)
	}

	proxy := httputil.NewSingleHostReverseProxy(upstream)

	proxy.Transport = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		log.Printf("Proxying request: %s %s%s", req.Method, req.Host, req.URL.RequestURI())

		originalPath := req.URL.Path

		switch originalPath {
		default:
		case "/api/tags":
			req.URL.Path = "/v1/models"
		case "/api/chat":
			req.URL.Path = "/v1/chat/completions"
		}
		// You can modify the request here if needed
		// For example, add a custom header
		if originalPath == "/api/tags" {

		}
		// req.Header.Set("X-Forwarded-For", "GoProxy")

		// Call the upstream server
		response, err := http.DefaultTransport.RoundTrip(req)
		if err != nil {
			return nil, fmt.Errorf("error making request to upstream: %w", err)
		}

		if originalPath == "/api/chat" {
			bodyBytes, err := io.ReadAll(response.Body)
			if err != nil {
				return nil, fmt.Errorf("error reading response body: %w", err)
			}
			defer response.Body.Close()

			jsonResponse, err := json.MarshalIndent(bodyBytes, "", "   ")
			if err != nil {
				return nil, fmt.Errorf("error marshaling response body to JSON: %w", err)
			}

			// Log the response body
			log.Printf("Response body: %s", jsonResponse)

			// Reassign the body to the response
			response.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
			response.ContentLength = int64(len(bodyBytes))
			response.Header.Set("Content-Length", strconv.Itoa(len(bodyBytes)))
		}

		if originalPath == "/api/tags" {
			openAIresponse := new(OpenAIResponse)
			if err = json.NewDecoder(response.Body).Decode(openAIresponse); err != nil {
				return nil, fmt.Errorf("error decoding response from upstream: %w", err)
			}
			defer response.Body.Close()

			ollama := new(OllamaResponse)
			ollama.Models = make([]OllamaModel, len(openAIresponse.Data))

			for i, data := range openAIresponse.Data {
				ollama.Models[i].Model = data.ID
				ollama.Models[i].Name = data.ID
			}

			jsonData, err := json.Marshal(ollama)
			if err != nil {
				return nil, fmt.Errorf("error marshaling response to JSON: %w", err)
			}

			// Reassign new body
			response.Body = io.NopCloser(bytes.NewBuffer(jsonData))
			response.ContentLength = int64(len(jsonData))
			response.Header.Set("Content-Length", strconv.Itoa(len(jsonData)))
		}

		return response, nil
	})

	log.Println("Listening on :1323 and proxying to", target)
	http.ListenAndServe(":1323", proxy)
}

type OpenAIModel struct {
	ID     string `json:"id"`
	Object string `json:"object"`
}

type OpenAIResponse struct {
	Data []OpenAIModel `json:"data"`
}

type OllamaModel struct {
	Name  string `json:"name"`
	Model string `json:"model"`
}

type OllamaResponse struct {
	Models []OllamaModel `json:"models"`
}
