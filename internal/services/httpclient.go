package services

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type RetryConfig struct {
	MaxRetries           int
	InitialDelay         time.Duration
	MaxDelay             time.Duration
	BackoffMultiplier    float64
	RetryableStatusCodes []int
}

var defaultRetryConfig = RetryConfig{
	MaxRetries:        3,
	InitialDelay:      100 * time.Millisecond,
	MaxDelay:          30 * time.Second,
	BackoffMultiplier: 2.0,
	RetryableStatusCodes: []int{
		http.StatusRequestTimeout,
		http.StatusTooManyRequests,
		http.StatusServiceUnavailable,
		http.StatusBadGateway,
		http.StatusGatewayTimeout,
	},
}

type HTTPClient struct {
	client *http.Client
	config RetryConfig
	cb     *CircuitBreaker
}

func NewHTTPClient(config RetryConfig, circuitBreaker *CircuitBreaker) *HTTPClient {
	if config.MaxRetries == 0 {
		config = defaultRetryConfig
	}
	if config.InitialDelay == 0 {
		config.InitialDelay = defaultRetryConfig.InitialDelay
	}
	if config.MaxDelay == 0 {
		config.MaxDelay = defaultRetryConfig.MaxDelay
	}
	if config.BackoffMultiplier == 0 {
		config.BackoffMultiplier = defaultRetryConfig.BackoffMultiplier
	}
	if len(config.RetryableStatusCodes) == 0 {
		config.RetryableStatusCodes = defaultRetryConfig.RetryableStatusCodes
	}

	return &HTTPClient{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		config: config,
		cb:     circuitBreaker,
	}
}

func (c *HTTPClient) Do(ctx context.Context, req *http.Request) (*http.Response, error) {
	if c.cb != nil {
		err := c.cb.Execute(ctx, func() error {
			_, err := c.doWithRetry(ctx, req)
			return err
		})
		if err != nil {
			return nil, err
		}
		return nil, nil
	}
	return c.doWithRetry(ctx, req)
}

func (c *HTTPClient) doWithRetry(ctx context.Context, req *http.Request) (*http.Response, error) {
	var lastErr error
	var resp *http.Response

	for attempt := 0; attempt <= c.config.MaxRetries; attempt++ {
		if attempt > 0 {
			delay := c.calculateBackoff(attempt)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
		}

		reqClone := req.Clone(ctx)
		resp, lastErr = c.client.Do(reqClone)

		if lastErr != nil {
			continue
		}

		if !c.isRetryableStatus(resp.StatusCode) {
			return resp, nil
		}

		if resp.Body != nil {
			resp.Body.Close()
		}
	}

	return nil, fmt.Errorf("max retries exceeded: %w", lastErr)
}

func (c *HTTPClient) calculateBackoff(attempt int) time.Duration {
	delay := float64(c.config.InitialDelay) * pow(c.config.BackoffMultiplier, float64(attempt-1))
	if delay > float64(c.config.MaxDelay) {
		return c.config.MaxDelay
	}
	return time.Duration(delay)
}

func pow(base, exp float64) float64 {
	result := 1.0
	for i := 0; i < int(exp); i++ {
		result *= base
	}
	return result
}

func (c *HTTPClient) isRetryableStatus(statusCode int) bool {
	for _, code := range c.config.RetryableStatusCodes {
		if statusCode == code {
			return true
		}
	}
	return false
}

func (c *HTTPClient) Get(ctx context.Context, url string, headers map[string]string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := c.Do(ctx, req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return io.ReadAll(resp.Body)
}

func (c *HTTPClient) Post(ctx context.Context, url string, body []byte, headers map[string]string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := c.Do(ctx, req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return io.ReadAll(resp.Body)
}

func (c *HTTPClient) Put(ctx context.Context, url string, body []byte, headers map[string]string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := c.Do(ctx, req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return io.ReadAll(resp.Body)
}

func (c *HTTPClient) Delete(ctx context.Context, url string, headers map[string]string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return nil, err
	}

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := c.Do(ctx, req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return io.ReadAll(resp.Body)
}

type BackoffStrategy func(attempt int) time.Duration

func ExponentialBackoff(initialDelay, maxDelay time.Duration, multiplier float64) BackoffStrategy {
	return func(attempt int) time.Duration {
		delay := float64(initialDelay) * pow(multiplier, float64(attempt))
		if delay > float64(maxDelay) {
			return maxDelay
		}
		return time.Duration(delay)
	}
}

func LinearBackoff(delay time.Duration) BackoffStrategy {
	return func(attempt int) time.Duration {
		return delay
	}
}

func ConstantBackoff(delay time.Duration) BackoffStrategy {
	return func(attempt int) time.Duration {
		return delay
	}
}
