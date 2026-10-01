package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"hwops/internal/domain"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	base := flag.String("url", "http://127.0.0.1:8080", "hwopsd base URL")
	manual := flag.String("manual", "testdata/manual-general.md", "synthetic Markdown fixture")
	question := flag.String("question", "蓝灯表示什么？", "question about the fixture")
	qa02 := flag.Bool("qa02", false, "run synthetic two-model/two-version device matching demonstration")
	flag.Parse()
	token := os.Getenv("HWOPS_API_TOKEN")
	if token == "" {
		return errors.New("HWOPS_API_TOKEN is required")
	}
	u, err := url.Parse(*base)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" ||
		(u.Scheme != "http" && u.Scheme != "https") {
		return errors.New("invalid server base URL")
	}
	client := &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	ctx, cancel := context.WithTimeout(context.Background(), 70*time.Second)
	defer cancel()
	call := func(method, path string, input, output any) error {
		return callJSON(ctx, client, token, method, strings.TrimRight(*base, "/")+path, input, output)
	}
	if *qa02 {
		return runQA02(ctx, call)
	}
	content, err := os.ReadFile(*manual)
	if err != nil {
		return err
	}
	var revision domain.Revision
	if err := call("POST", "/v1/knowledge/revisions", domain.RevisionInput{
		Title: "合成演示手册", Source: "fixture://manual-general.md", Content: string(content),
		Applicability: domain.Applicability{Scope: "GENERAL"},
	}, &revision); err != nil {
		return err
	}
	if err := call("POST", "/v1/knowledge/revisions/"+revision.ID+"/publication",
		map[string]string{"decision": "PUBLISH"}, &revision); err != nil {
		return err
	}
	var conversation domain.Conversation
	if err := call("POST", "/v1/conversations", struct{}{}, &conversation); err != nil {
		return err
	}
	var response domain.Response
	if err := call("POST", "/v1/conversations/"+conversation.ID+"/messages",
		map[string]string{"text": *question}, &response); err != nil {
		return err
	}
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for response.Status == "QUEUED" || response.Status == "RUNNING" {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
		if err := call("GET", "/v1/responses/"+response.ID, nil, &response); err != nil {
			return err
		}
	}
	fmt.Printf("data_mode=%s status=%s response_id=%s\n", response.DataMode, response.Status, response.ID)
	if response.Error != nil {
		return fmt.Errorf("%s: %s", response.Error.Code, response.Error.Message)
	}
	if response.Status != "ANSWERED" || len(response.Citations) == 0 {
		return fmt.Errorf("no cited answer: %v", response.Gaps)
	}
	fmt.Println(response.Answer)
	for _, citation := range response.Citations {
		var original struct {
			domain.Fragment
			PublicationStatus string `json:"publication_status"`
		}
		if err := call("GET", citation.URL, nil, &original); err != nil {
			return err
		}
		if original.ID != citation.FragmentID || original.ContentHash != citation.ContentHash {
			return errors.New("citation does not match original fragment")
		}
		fmt.Printf("\n引用：%s / %s / 第 %d–%d 行 / %s\n%s%s\n原文：%s\n",
			citation.Title, citation.Section, original.StartLine, original.EndLine,
			original.PublicationStatus, strings.TrimRight(*base, "/"), citation.URL, original.Content)
	}
	return nil
}

func callJSON(ctx context.Context, client *http.Client, token, method, target string, input, output any) error {
	var body io.Reader
	if input != nil {
		raw, err := json.Marshal(input)
		if err != nil {
			return err
		}
		body = bytes.NewReader(raw)
	}
	request, err := http.NewRequestWithContext(ctx, method, target, body)
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("%s failed: HTTP %d", method, response.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(response.Body, 2*1024*1024)).Decode(output)
}
