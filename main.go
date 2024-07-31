package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
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
	Username            string
	Password            string
	ClientID            string
	ClientSecret        string
	UserAgent           string
	SkipCommentIDs      []string
	SkipSubreddits      []string
	Before              time.Time
	MaxScore            int
	ReplacementComment  string
	DryRun              bool
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

func (c *Comment) Created() time.Time {
	return time.Unix(int64(c.Source.CreatedUTC), 0)
}

func (c *Comment) Fullname() string {
	return "t1_" + c.ID
}

func (c *Comment) ShouldSkip(config *Config) bool {
	for _, id := range config.SkipCommentIDs {
		if id == c.ID {
			log.Printf("Skipping due to `skip_comment_ids` filter")
			return true
		}
	}
	for _, subreddit := range config.SkipSubreddits {
		if subreddit == c.Subreddit {
			log.Printf("Skipping due to `skip_subreddits` filter")
			return true
		}
	}
	if c.Created().After(config.Before) {
		log.Printf("Skipping due to `before` filter (%s)", config.Before)
		return true
	}
	if c.Source.Score > int64(config.MaxScore) {
		log.Printf("Skipping due to `max_score` filter (%d)", config.MaxScore)
		return true
	}
	return false
}

func (c *Comment) Delete(client *http.Client, accessToken string, config *Config) {
	log.Println("Deleting...")

	if c.ShouldSkip(config) || config.DryRun {
		return
	}

	data := url.Values{}
	data.Set("id", c.Fullname())

	req, err := http.NewRequest("POST", "https://oauth.reddit.com/api/del", strings.NewReader(data.Encode()))
	if err != nil {
		log.Printf("Failed to create request: %v", err)
		return
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("User-Agent", config.UserAgent)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Failed to delete comment: %v", err)
		return
	}
	defer resp.Body.Close()
}

func (c *Comment) Edit(client *http.Client, accessToken string, config *Config) {
	log.Println("Editing...")

	if c.ShouldSkip(config) || config.DryRun {
		return
	}

	data := url.Values{}
	data.Set("thing_id", c.Fullname())
	data.Set("text", config.ReplacementComment)

	req, err := http.NewRequest("POST", "https://oauth.reddit.com/api/editusertext?raw_json=1", strings.NewReader(data.Encode()))
	if err != nil {
		log.Printf("Failed to create request: %v", err)
		return
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("User-Agent", config.UserAgent)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Failed to edit comment: %v", err)
		return
	}
	defer resp.Body.Close()

	var res map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		log.Printf("Failed to decode response: %v", err)
		return
	}

	if _, ok := res["jquery"]; ok {
		log.Printf("Edited successfully.")
	} else {
		log.Printf("Failed to edit: %v", res)
	}
}

func List(client *http.Client, config *Config) <-chan Comment {
	out := make(chan Comment)

	go func() {
		defer close(out)
		log.Println("Fetching comments...")
		var lastSeen string

		for {
			queryParams := ""
			if lastSeen != "" {
				queryParams = "?after=" + lastSeen
			}

			uri := fmt.Sprintf("https://reddit.com/user/%s/comments.json%s", config.Username, queryParams)

			req, err := http.NewRequest("GET", uri, nil)
			if err != nil {
				log.Printf("Failed to create request: %v", err)
				return
			}

			req.Header.Set("User-Agent", config.UserAgent)

			resp, err := client.Do(req)
			if err != nil {
				log.Printf("Failed to fetch comments: %v", err)
				return
			}
			defer resp.Body.Close()

			var res Response
			if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
				log.Printf("Failed to decode response: %v", err)
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

	// Decode response
	var res AccessTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return "", err
	}

	// Check for errors in the response
	if res.Error != "" {
		return "", errors.New(res.ErrorDesc)
	}

	return res.AccessToken, nil
}

func main() {
	client := &http.Client{}

	config := &Config{
		Username:           os.Getenv("REDDIT_USERNAME"),
		Password:           os.Getenv("REDDIT_PASSWORD"),
		ClientID:           os.Getenv("REDDIT_CLIENT_ID"),
		ClientSecret:       os.Getenv("REDDIT_CLIENT_SECRET"),
		UserAgent:          os.Getenv("REDDIT_USER_AGENT"),
		SkipCommentIDs:     []string{},
		SkipSubreddits:     []string{},
		Before:             time.Now().AddDate(-os.Getenv("REDDIT_YEARS_BACK"),, 0, 0), // 11 year ago
		MaxScore:           0,
		ReplacementComment: "",
		DryRun:             os.Getenv("REDDIT_DRY_RUN"),
	}

	// Get access token
	accessToken, err := newAccessToken(config)
	if err != nil {
		log.Fatalf("Failed to obtain access token: %v", err)
	}

	// List and process comments
	for comment := range List(client, config) {
		comment.Edit(client, accessToken, config)
		comment.Delete(client, accessToken, config)
		fmt.Print("Sleeping 15 seconds\n") // This is mostly to prevent getting throttled
		time.Sleep(15 * time.Second) 
	}
}