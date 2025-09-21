package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"os"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	baseURL = "https://api-my.te.eg"
)

// API response structures
type ResponseHeader struct {
	RetCode string `json:"retCode"`
}

type AuthResponse struct {
	Header ResponseHeader `json:"header"`
	Body   struct {
		Token      string `json:"token"`
		Subscriber struct {
			SubscriberID string `json:"subscriberId"`
		} `json:"subscriber"`
	} `json:"body"`
}

type OfferingsResponse struct {
	Header ResponseHeader `json:"header"`
	Body   struct {
		OfferingList []struct {
			MainOfferingID string `json:"mainOfferingId"`
		} `json:"offeringList"`
	} `json:"body"`
}

type QuotaDetail struct {
	Used   float64 `json:"used"`
	Total  float64 `json:"total"`
	Remain float64 `json:"remain"`
}

type QuotaResponse struct {
	Header ResponseHeader  `json:"header"`
	Body   []QuotaDetail `json:"body"`
}

// WeQuotaChecker holds the state for our checker
type WeQuotaChecker struct {
	landlineNumber string
	password       string
	accountID      string
	client         *http.Client
	metrics        *Metrics
}

// Metrics holds our Prometheus gauges
type Metrics struct {
	remainingGB     prometheus.Gauge
	usagePercentage prometheus.Gauge
	totalGB         prometheus.Gauge // <-- ADDED
}

// NewWeQuotaChecker initializes the checker and its metrics
func NewWeQuotaChecker(landlineNumber, password string) (*WeQuotaChecker, error) {
	if landlineNumber == "" || password == "" {
		return nil, fmt.Errorf("landline number and password are required")
	}

	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create cookie jar: %w", err)
	}

	// Create a new non-global registry for our metrics
	reg := prometheus.NewRegistry()
	metrics := &Metrics{
		remainingGB: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "we_quota_remaining_gb",
			Help: "Remaining internet quota in GB.",
		}),
		usagePercentage: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "we_quota_usage_percentage",
			Help: "Internet quota usage percentage.",
		}),
		// v-- ADDED --v
		totalGB: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "we_quota_total_gb",
			Help: "Total internet quota in GB.",
		}),
		// ^-- ADDED --^
	}
	// Register all metrics with our custom registry
	reg.MustRegister(metrics.remainingGB, metrics.usagePercentage, metrics.totalGB)

	http.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))

	return &WeQuotaChecker{
		landlineNumber: landlineNumber,
		password:       password,
		accountID:      "FBB" + landlineNumber[1:],
		client:         &http.Client{Timeout: 20 * time.Second, Jar: jar},
		metrics:        metrics,
	}, nil
}

// genericRequest handles creating and executing API requests with all required headers
func (w *WeQuotaChecker) genericRequest(method, url string, payload, response interface{}, token ...string) error {
	var bodyReader io.Reader
	if payload != nil {
		jsonBody, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("failed to marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(jsonBody)
	}

	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		return fmt.Errorf("failed to create new request: %w", err)
	}

	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	req.Header.Set("channelId", "702")
	req.Header.Set("isCoporate", "false")
	req.Header.Set("isMobile", "false")
	req.Header.Set("isSelfcare", "true")
	req.Header.Set("languageCode", "en-US")
	if len(token) > 0 && token[0] != "" {
		req.Header.Set("csrftoken", token[0])
	}

	resp, err := w.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if err := json.NewDecoder(resp.Body).Decode(response); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	return nil
}

func (w *WeQuotaChecker) runCheck() {
	// 1. Authenticate
	var authResponse AuthResponse
	authPayload := map[string]string{"acctId": w.accountID, "password": w.password, "appLocale": "en-US"}
	err := w.genericRequest("POST", baseURL+"/echannel/service/besapp/base/rest/busiservice/v1/auth/userAuthenticate", authPayload, &authResponse)
	if err != nil || authResponse.Header.RetCode != "0" {
		log.Printf("Error during authentication: %v, retCode: %s", err, authResponse.Header.RetCode)
		return
	}
	token := authResponse.Body.Token
	subscriberID := authResponse.Body.Subscriber.SubscriberID

	// 2. Get Subscribed Offerings
	var offeringsResponse OfferingsResponse
	offeringsPayload := map[string]string{"msisdn": w.accountID, "numberServiceType": "FBB", "groupId": ""}
	err = w.genericRequest("POST", baseURL+"/echannel/service/besapp/base/rest/busiservice/cz/v1/auth/getSubscribedOfferings", offeringsPayload, &offeringsResponse, token)
	if err != nil || offeringsResponse.Header.RetCode != "0" {
		log.Printf("Error getting subscribed offerings: %v, retCode: %s", err, offeringsResponse.Header.RetCode)
		return
	}
	if len(offeringsResponse.Body.OfferingList) == 0 {
		log.Println("Error: No offerings found")
		return
	}
	offerID := offeringsResponse.Body.OfferingList[0].MainOfferingID

	// 3. Get Quota Details
	var quotaResponse QuotaResponse
	quotaPayload := map[string]string{"subscriberId": subscriberID, "mainOfferId": offerID}
	err = w.genericRequest("POST", baseURL+"/echannel/service/besapp/base/rest/busiservice/cz/cbs/bb/queryFreeUnit", quotaPayload, &quotaResponse, token)
	if err != nil || quotaResponse.Header.RetCode != "0" {
		log.Printf("Error getting quota details: %v, retCode: %s", err, quotaResponse.Header.RetCode)
		return
	}
	if len(quotaResponse.Body) == 0 {
		log.Println("Error: No quota details found")
		return
	}
	quota := quotaResponse.Body[0]

	// 4. Update Metrics
	usagePercentage := 0.0
	if quota.Total > 0 {
		usagePercentage = (quota.Used / quota.Total) * 100
	}
	w.metrics.remainingGB.Set(quota.Remain)
	w.metrics.usagePercentage.Set(usagePercentage)
	w.metrics.totalGB.Set(quota.Total) // <-- ADDED

	// Updated log message
	log.Printf("Quota check successful. Remaining: %.2f GB / %.2f GB, Usage: %.2f%%", quota.Remain, quota.Total, usagePercentage)
}

func main() {
	landlineNumber := os.Getenv("LANDLINE_NUMBER")
	password := os.Getenv("PASSWORD")
	intervalStr := os.Getenv("INTERVAL")

	if landlineNumber == "" || password == "" || intervalStr == "" {
		log.Fatal("Error: LANDLINE_NUMBER, PASSWORD, and INTERVAL environment variables must be set.")
	}

	interval, err := time.ParseDuration(intervalStr)
	if err != nil {
		log.Fatalf("Error parsing INTERVAL: %v\n", err)
	}

	checker, err := NewWeQuotaChecker(landlineNumber, password)
	if err != nil {
		log.Fatalf("Error creating quota checker: %v\n", err)
	}

	go func() {
		log.Println("Starting Prometheus metrics server on :2222")
		if err := http.ListenAndServe(":2222", nil); err != nil {
			log.Fatalf("Failed to start metrics server: %v", err)
		}
	}()

	log.Println("Performing initial quota check...")
	checker.runCheck()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for range ticker.C {
		log.Println("Performing scheduled quota check...")
		checker.runCheck()
	}
}