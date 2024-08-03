package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type AccessTokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
	Scope       string `json:"scope"`
	TokenType   string `json:"token_type"`
	Error       string `json:"error,omitempty"`
	ErrorDesc   string `json:"error_description,omitempty"`
}

type Config struct {
	Username           string
	Password           string
	ClientID           string
	ClientSecret       string
	UserAgent          string
	SkipCommentIDs     []string
	SkipSubreddits     []string
	Before             time.Time
	MaxScore           int
	ReplacementComment string
	DryRun             bool
}

type RawConfig struct {
	Username           string   `json:"username"`
	Password           string   `json:"password"`
	ClientID           string   `json:"ClientID"`
	ClientSecret       string   `json:"ClientSecret"`
	UserAgent          string   `json:"UserAgent"`
	SkipCommentIDs     []string `json:"SkipCommentIDs"`
	SkipSubreddits     []string `json:"SkipSubreddits"`
	Before             string   `json:"Before"`
	MaxScore           int      `json:"MaxScore"`
	ReplacementComment string   `json:"ReplacementComment"`
	DryRun             bool     `json:"DryRun"`
}

type Comment struct {
	ID        string
	Body      string
	Permalink string
	Subreddit string
	Source    Source
}

type Source struct {
	Score      int64
	CreatedUTC float64
	CanGild    bool
	Date       time.Time
}

type ResponseData struct {
	Children []Child
	After    string
	Before   string
}

type Child struct {
	Data Comment
}

type Response struct {
	Data ResponseData
}

// EnvVar or Json Value
func getEnvOrDefault(envVar, defaultValue string) string {
	if value, exists := os.LookupEnv(envVar); exists {
		return value
	}
	return defaultValue
}

func configLoader(filePath string) (*Config, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open config file: %v", err)
	}
	defer file.Close()

	var rawConfig RawConfig
	if err := json.NewDecoder(file).Decode(&rawConfig); err != nil {
		return nil, fmt.Errorf("failed to decode config file: %v", err)
	}

	before, err := time.Parse(time.RFC3339, rawConfig.Before)
	if err != nil {
		return nil, fmt.Errorf("failed to parse 'Before' date: %v", err)
	}

	config := &Config{
		Username:           rawConfig.Username,
		Password:           getEnvOrDefault("REDDIT_PASSWORD", rawConfig.Password),
		ClientID:           rawConfig.ClientID,
		ClientSecret:       getEnvOrDefault("REDDIT_CLIENT_SECRET", rawConfig.ClientSecret),
		UserAgent:          rawConfig.UserAgent,
		SkipCommentIDs:     rawConfig.SkipCommentIDs,
		SkipSubreddits:     rawConfig.SkipSubreddits,
		Before:             before,
		MaxScore:           rawConfig.MaxScore,
		ReplacementComment: rawConfig.ReplacementComment,
		DryRun:             rawConfig.DryRun,
	}

	return config, nil
}

func (c *Comment) Created() time.Time {
	return time.Unix(int64(c.Source.CreatedUTC), 0)
}

func (c *Comment) Fullname() string {
	return "t1_" + c.ID
}

func (c *Comment) ShouldSkip(config *Config) bool {
	for _, id := range config.SkipCommentIDs {
		if id == c.ID {
			fmt.Printf("Skipping due to `skip_comment_ids` filter\n")
			return true
		}
	}
	for _, subreddit := range config.SkipSubreddits {
		fmt.Printf("Subreddit: %v\n", subreddit)
		if subreddit == c.Subreddit {
			fmt.Printf("Skipping due to `skip_subreddits` filter\n")
			return true
		}
	}
	if c.Created().After(config.Before) {
		fmt.Printf("Skipping due to `before` filter (%s)\n", config.Before)
		return true
	}
	if c.Source.Score > int64(config.MaxScore) {
		fmt.Printf("Skipping due to `max_score` filter (%d)\n", config.MaxScore)
		return true
	}
	return false
}

func LoadConfig(filename string) (*Config, error) {
	// Open File
	file, err := os.Open(filename)
	if err != nil {
		fmt.Printf("Failed to open config file: %v", err)
		return nil, err
	}
	defer file.Close()

	// Read File
	byteValue, err := ioutil.ReadAll(file)
	if err != nil {
		fmt.Printf("Failed to read config file: %v", err)
		return nil, err
	}

	var config Config
	// Unmarshal JSON into config struct
	if err := json.Unmarshal(byteValue, &config); err != nil {
		return nil, fmt.Errorf("Failed to unmarshal config into struct: %v", err)
	}

	// Override sensitive fields with environment variables
	if password := os.Getenv("REDDIT_PASSWORD"); password != "" {
		config.Password = password
	} else {
		return nil, fmt.Errorf("Reddit password not set")
	}

	if clientSecret := os.Getenv("REDDIT_CLIENT_SECRET"); clientSecret != "" {
		config.ClientSecret = clientSecret
	} else {
		return nil, fmt.Errorf("Reddit Client Secret not set")
	}

	// Handle yearsBack from environment variable
	yearsBackStr := os.Getenv("REDDIT_YEARS_BACK")
	yearsBack, err := strconv.Atoi(yearsBackStr)
	if err != nil {
		if yearsBackStr != "" {
			fmt.Printf("Invalid value for REDDIT_YEARS_BACK: %v", err)
			return nil, err
		}
		yearsBack = 11 // Default to 11 if not set or invalid
	}

	config.Before = time.Now().AddDate(-yearsBack, 0, 0)

	// Handle DryRun
	config.DryRun = os.Getenv("REDDIT_DRY_RUN") == "true"

	return &config, nil
}

func (c *Comment) Delete(client *http.Client, accessToken string, config *Config) {

	if c.ShouldSkip(config) || config.DryRun {
		fmt.Println("dryrun set or item set to be skipped, skipping deletion.")
		return
	}

	fmt.Println("Deleting...")
	data := url.Values{}
	data.Set("id", c.Fullname())

	req, err := http.NewRequest("POST", "https://oauth.reddit.com/api/del", strings.NewReader(data.Encode()))
	if err != nil {
		fmt.Printf("Failed to send delete request: %v\n", err)
		return
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("User-Agent", config.UserAgent)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("Failed to delete comment: %v\n", err)
		return
	}
	defer resp.Body.Close()
}

func (c *Comment) Edit(client *http.Client, accessToken string, config *Config) {
	if c.ShouldSkip(config) || config.DryRun {
		return
	}

	fmt.Println("Editing...")

	data := url.Values{}
	data.Set("thing_id", c.Fullname())
	data.Set("text", config.ReplacementComment)

	req, err := http.NewRequest("POST", "https://oauth.reddit.com/api/editusertext?raw_json=1", strings.NewReader(data.Encode()))
	if err != nil {
		fmt.Printf("Failed to create request: %v\n", err)
		return
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("User-Agent", config.UserAgent)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("Failed to edit comment: %v\n", err)
		return
	}
	defer resp.Body.Close()

	var res map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		fmt.Printf("Failed to decode response: %v\n", err)
		return
	}

	if _, ok := res["jquery"]; ok {
		fmt.Printf("Edited successfully.\n")
	} else {
		fmt.Printf("Failed to edit: %v\n", res)
	}
}

func List(client *http.Client, config *Config) <-chan Comment {
	out := make(chan Comment)

	go func() {
		defer close(out)
		fmt.Println("Fetching comments...")
		var lastSeen string

		for {
			queryParams := ""
			if lastSeen != "" {
				queryParams = "?after=" + lastSeen
			}

			uri := fmt.Sprintf("https://reddit.com/user/%s/comments.json%s", config.Username, queryParams)

			req, err := http.NewRequest("GET", uri, nil)
			if err != nil {
				fmt.Printf("Failed to create request: %v", err)
				return
			}

			req.Header.Set("User-Agent", config.UserAgent)

			resp, err := client.Do(req)
			if err != nil {
				fmt.Printf("Failed to fetch comments: %v", err)
				return
			}
			defer resp.Body.Close()

			var res Response
			if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
				fmt.Printf("Failed to decode response: %v", err)
				return
			}

			for _, child := range res.Data.Children {
				out <- child.Data
			}

			if len(res.Data.Children) == 0 || res.Data.After == "" {
				break
			}

			lastSeen = res.Data.After
		}
	}()

	return out
}

func newAccessToken(config *Config) (string, error) {
	// Prepare form data
	form := url.Values{}
	form.Add("grant_type", "password")
	form.Add("username", config.Username)
	form.Add("password", config.Password)

	// Prepare request
	req, err := http.NewRequest("POST", "https://www.reddit.com/api/v1/access_token", strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}

	req.SetBasicAuth(config.ClientID, config.ClientSecret)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", config.UserAgent)

	// Send request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	// Read the response body
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %v", err)
	}

	// Try to decode response as JSON
	var res AccessTokenResponse
	if err := json.Unmarshal(body, &res); err != nil {
		// Log the response body for further inspection
		return "", fmt.Errorf("unable to decode response: %v\nResponse body: %s", err, body)
	}

	// Check for errors in the response
	if res.Error != "" {
		return "", fmt.Errorf("error in the response: %s", res.ErrorDesc)
	}

	return res.AccessToken, nil
}

func main() {
	// Load config
	config, err := configLoader("config.json")
	if err != nil {
		fmt.Errorf("Error loading config: %v\n", err)
	}

	// Get access token
	accessToken, err := newAccessToken(config)
	if err != nil {
		fmt.Errorf("Failed to obtain access token: %v\n", err)
	}

	client := &http.Client{}
	// List and process comments
	for comment := range List(client, config) {
		// Two-step approach to cleaning: edit first, then delete
		comment.Edit(client, accessToken, config)
		comment.Delete(client, accessToken, config)

		// Sleep to avoid throttling
		//fmt.Print("Sleeping 15 seconds\n")
		//time.Sleep(15 * time.Second)
	}
}
