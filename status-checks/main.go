package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/trade-tariff/trade-tariff-lambdas-status-checks/logger"

	"github.com/BurntSushi/toml"
	"github.com/aws/aws-lambda-go/lambda"
)

// var defaultConfig = map[string]string{
// 	"STATUS_BUCKET": "trade-tariff-status-checks-844815912454",
// 	"DEBUG":         "false",
// }

type Result struct {
	Duration   time.Duration
	StatusCode int
}

type Status string

const (
	ACTIVE   Status = "ACTIVE"
	INACTIVE Status = "INACTIVE"
	DEGRADED Status = "DEGRADED"
)

type AllResponseTimes struct {
	Applications []AppResponseTimes `json:"applications"`
	LastUpdated  time.Time          `json:"last_updated"`
}

type AppResponseTimes struct {
	TotalRequests int             `json:"total_requests"`
	Application   string          `json:"application"`
	URL           string          `json:"url"`
	P90           float64         `json:"p90"`
	AllTimes      []time.Duration `json:"-"`
	ErrorCount    int             `json:"error_count"`
	SuccessCount  int             `json:"success_count"`
	Status        Status          `json:"status"`
}

//go:embed applications.toml
var configFS embed.FS

type Config struct {
	Applications []AppConfig `toml:"application"`
}

type AppConfig struct {
	Name                           string  `toml:"name"`
	URL                            string  `toml:"url"`
	AuthHeaderValue                string  `toml:"authHeaderValue"`
	AuthHeader                     string  `toml:"authHeader"`
	Verb                           string  `toml:"verb"`
	Data                           string  `toml:"data"`
	ErrorHighWatermarkPercentage   int     `toml:"errorHighWatermarkPercentage"`
	ErrorMediumWatermarkPercentage int     `toml:"errorMediumWatermarkPercentage"`
	ErrorLowWatermarkPercentage    int     `toml:"errorLowWatermarkPercentage"`
	P90HighWatermarkSeconds        float64 `toml:"p90HighWatermarkSeconds"`
	P90MediumWatermarkSeconds      float64 `toml:"p90MediumWatermarkSeconds"`
	P90LowWatermarkSeconds         float64 `toml:"p90LowWatermarkSeconds"`
}

type CloudWatchEvent struct {
	DetailType string `json:"detail-type"`
}

func main() {
	if os.Getenv("AWS_LAMBDA_FUNCTION_VERSION") != "" {
		lambda.Start(execute)
	} else {
		execute(context.TODO(), CloudWatchEvent{DetailType: "Local Event"})
	}
}

func execute(ctx context.Context, event CloudWatchEvent) {
	logger.Log.Info(fmt.Sprintf("Starting status checks from %s", event.DetailType))
	applications := initializeApplications()

	var wg sync.WaitGroup

	allResponseTimes := AllResponseTimes{
		Applications: make([]AppResponseTimes, 0, len(applications)),
		LastUpdated:  time.Now(),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	for _, config := range applications {
		wg.Add(1)
		go func(config AppConfig) {
			defer wg.Done()
			results := collectResults(ctx, 5, config)
			allResponseTimes.Applications = append(allResponseTimes.Applications, results)
		}(config)
	}

	wg.Wait()

	jsonData, err := json.Marshal(allResponseTimes)

	if err != nil {
		logger.Log.Fatal("Error occurred while marshalling JSON")
	}

	logger.Log.Info(string(jsonData))
}

func collectResults(ctx context.Context, numWorkers int, config AppConfig) AppResponseTimes {
	results := make(chan Result, 100)

	var wg sync.WaitGroup

	wg.Add(numWorkers)
	for i := 0; i < numWorkers; i++ {
		go func() {
			defer wg.Done()
			worker(ctx, config, results)
		}()
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	var errorCount int
	var successCount int
	var allTimes []time.Duration

	for r := range results {
		if r.StatusCode >= 400 {
			errorCount++
		} else {
			successCount++
		}
		allTimes = append(allTimes, r.Duration)
	}

	p90 := computeP90(allTimes)
	status := computeStatus(config, p90, errorCount)

	return AppResponseTimes{
		TotalRequests: len(allTimes),
		Application:   config.Name,
		URL:           config.URL,
		P90:           p90,
		AllTimes:      allTimes,
		ErrorCount:    errorCount,
		SuccessCount:  successCount,
		Status:        status,
	}
}

func worker(ctx context.Context, config AppConfig, results chan Result) {
	client := &http.Client{}
	for {
		select {
		case <-ctx.Done():
			return
		default:
			var response *http.Response
			var err error

			start := time.Now()
			if config.Verb == "POST" {
				response, err = post(client, config)
			} else {
				response, err = get(client, config)
			}

			if err != nil {
				logger.Log.Error(
					"Error occurred while making request",
					logger.String("url", config.URL),
					logger.String("error", err.Error()),
				)
				continue
			}
			response.Body.Close()
			duration := time.Since(start)
			results <- Result{
				Duration:   duration,
				StatusCode: response.StatusCode,
			}
		}
	}
}

func get(client *http.Client, config AppConfig) (*http.Response, error) {
	req, err := http.NewRequest(
		"GET",
		config.URL,
		nil,
	)

	if err != nil {
		return nil, err
	}

	setAuthHeader(req, config)
	req.Header.Set("User-Agent", "Status-Checks")

	return client.Do(req)
}

func post(client *http.Client, config AppConfig) (*http.Response, error) {
	req, err := http.NewRequest(
		"POST",
		config.URL,
		strings.NewReader(config.Data),
	)

	if err != nil {
		return nil, err
	}

	setAuthHeader(req, config)
	req.Header.Set("User-Agent", "Status-Checks")

	return client.Do(req)
}

func initializeApplications() []AppConfig {
	var config Config

	configData, err := configFS.ReadFile("applications.toml")

	if err != nil {
		logger.Log.Fatal("Error occurred while reading config file")
	}

	if _, err := toml.Decode(string(configData), &config); err != nil {
		logger.Log.Fatal(
			"Error occurred while decoding config file",
			logger.String("error", err.Error()),
		)
	}

	return config.Applications
}

func setAuthHeader(req *http.Request, config AppConfig) {
	var authHeader string
	var authHeaderValue string

	if config.AuthHeader != "" && config.AuthHeaderValue != "" {
		authHeader = config.AuthHeader

		if strings.HasPrefix(config.AuthHeaderValue, "env:") {
			authHeaderValue = os.Getenv(strings.TrimPrefix(config.AuthHeaderValue, "env:"))
		}

		req.Header.Set(authHeader, authHeaderValue)
	}
}

func computeP90(allTimes []time.Duration) float64 {
	if len(allTimes) == 0 {
		return 0.0
	}

	sort.Slice(allTimes, func(i, j int) bool {
		return allTimes[i] < allTimes[j]
	})

	p90Index := int(math.Ceil(float64(len(allTimes))*0.9)) - 1

	if p90Index >= len(allTimes) {
		p90Index = len(allTimes) - 1
	}

	p90 := allTimes[p90Index].Seconds()

	return p90
}

func computeStatus(config AppConfig, p90 float64, errorCount int) Status {
	if errorCount > config.ErrorHighWatermarkPercentage {
		return INACTIVE
	}

	if errorCount > config.ErrorMediumWatermarkPercentage {
		return DEGRADED
	}

	if errorCount > config.ErrorLowWatermarkPercentage {
		return ACTIVE
	}

	if p90 > config.P90HighWatermarkSeconds {
		return INACTIVE
	}

	if p90 > config.P90MediumWatermarkSeconds {
		return DEGRADED
	}

	if p90 > config.P90LowWatermarkSeconds {
		return ACTIVE
	}

	return ACTIVE
}