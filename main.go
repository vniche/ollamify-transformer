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
	"strings"

	"github.com/gin-gonic/gin"
)

const backendBaseURL = "http://localhost:8080" // Replace with your upstream server

func main() {
	r := gin.Default()

	// Apply reverse proxy middleware to all routes
	r.Use(ReverseProxyMiddleware(backendBaseURL))

	// Fallback route (won't be hit if middleware proxies correctly)
	r.NoRoute(func(c *gin.Context) {
		c.String(http.StatusNotFound, "Not found")
	})

	log.Println("Starting proxy server on :1323")
	r.Run(":1323")
}

// ReverseProxyMiddleware returns a gin.HandlerFunc that proxies the request to the target backend.
func ReverseProxyMiddleware(target string) gin.HandlerFunc {
	targetURL, err := url.Parse(target)
	if err != nil {
		log.Fatalf("Invalid proxy target URL: %v", err)
	}

	return func(c *gin.Context) {
		director := func(req *http.Request) {
			req.URL.Scheme = targetURL.Scheme
			req.URL.Host = targetURL.Host
			path := c.Request.URL.Path
			switch path {
			default:
			case "/api/tags":
				path = "/v1/models"
			case "/api/chat":
				path = "/v1/chat/completions"
			}
			req.URL.Path = singleJoiningSlash(targetURL.Path, path)
			req.URL.RawQuery = c.Request.URL.RawQuery
			req.Header = c.Request.Header.Clone()
			req.Header.Set("X-Forwarded-Path", c.Request.URL.Path)
		}

		// Custom transport for handling stream proxying
		transport := http.DefaultTransport

		proxy := &httputil.ReverseProxy{
			Director: director,
			Transport: roundTripperStreamAdapter{
				base: transport,
				ctx:  c,
			},
			ModifyResponse: nil,
			ErrorHandler: func(rw http.ResponseWriter, req *http.Request, err error) {
				log.Printf("Proxy error: %v", err)
				c.AbortWithStatusJSON(http.StatusBadGateway, gin.H{"error": "Proxy error"})
			},
		}

		proxy.ServeHTTP(c.Writer, c.Request)

		// Important: don't call next middleware
		c.Abort()
	}
}

// roundTripperStreamAdapter uses gin.Context.Stream to stream the response.
type roundTripperStreamAdapter struct {
	base http.RoundTripper
	ctx  *gin.Context
}

func (r roundTripperStreamAdapter) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := r.base.RoundTrip(req)
	if err != nil {
		return nil, err
	}

	// Use the forwarded path from the request header for conditional logic
	switch req.Header.Get("X-Forwarded-Path") {
	default:
	case "/api/tags":
		openAIresponse := new(OpenAIResponse)
		if err = json.NewDecoder(resp.Body).Decode(openAIresponse); err != nil {
			return nil, fmt.Errorf("error decoding response from upstream: %w", err)
		}
		defer resp.Body.Close()

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
		resp.Body = io.NopCloser(bytes.NewBuffer(jsonData))
		resp.ContentLength = int64(len(jsonData))
		resp.Header.Set("Content-Length", strconv.Itoa(len(jsonData)))

		return resp, nil
	}

	// Handle stream manually if "Transfer-Encoding: chunked"
	if strings.Contains(strings.ToLower(resp.Header.Get("Transfer-Encoding")), "chunked") {
		r.ctx.Status(resp.StatusCode)
		for k, v := range resp.Header {
			for _, vv := range v {
				r.ctx.Writer.Header().Add(k, vv)
			}
		}
		r.ctx.Stream(func(w io.Writer) bool {
			_, err := io.Copy(w, resp.Body)
			defer resp.Body.Close()
			return err == nil
		})
		return nil, nil // short-circuit, response already sent
	}

	return resp, nil
}

func singleJoiningSlash(a, b string) string {
	aslash := strings.HasSuffix(a, "/")
	bslash := strings.HasPrefix(b, "/")
	switch {
	case aslash && bslash:
		return a + b[1:]
	case !aslash && !bslash:
		return a + "/" + b
	default:
		return a + b
	}
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
