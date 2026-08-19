package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// certadmin is an admin CLI for managing the certification platform.
// It can list submissions, dispatch tasks, check consistency, and
// view audit logs by calling the gateway's REST API.

func main() {
	serverURL := flag.String("server", "http://127.0.0.1:52661", "gateway server URL")
	flag.Parse()

	if len(flag.Args()) == 0 {
		printUsage()
		os.Exit(1)
	}

	cmd := flag.Arg(0)
	args := flag.Args()[1:]

	client := &http.Client{Timeout: 30 * time.Second}

	switch cmd {
	case "submissions":
		listSubmissions(client, *serverURL)
	case "dispatch":
		if len(args) < 1 {
			fmt.Fprintln(os.Stderr, "usage: certadmin dispatch <submission_id>")
			os.Exit(1)
		}
		dispatchSubmission(client, *serverURL, args[0])
	case "summary":
		if len(args) < 1 {
			fmt.Fprintln(os.Stderr, "usage: certadmin summary <submission_id>")
			os.Exit(1)
		}
		getSummary(client, *serverURL, args[0])
	case "consistency":
		if len(args) < 1 {
			fmt.Fprintln(os.Stderr, "usage: certadmin consistency <submission_id>")
			os.Exit(1)
		}
		checkConsistency(client, *serverURL, args[0])
	case "audit":
		listAudit(client, *serverURL)
	case "gateway":
		gatewayStatus(client, *serverURL)
	case "backlog":
		backlogStats(client, *serverURL)
	case "create-submission":
		createSubmission(client, *serverURL, args)
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", cmd)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Fprintln(os.Stderr, `Usage: certadmin [flags] <command> [args]

Commands:
  submissions              List all submissions
  dispatch <submission_id> Dispatch a submission to agencies
  summary <submission_id>  Get summary for a submission
  consistency <submission_id> Check consistency for a submission
  audit                    List recent audit logs
  gateway                   Show gateway upstream status
  backlog                   Show backlog statistics
  create-submission         Create a test submission

Flags:
  -server <url>  Gateway server URL (default http://127.0.0.1:52661)`)
}

func doGet(client *http.Client, url string) {
	resp, err := client.Get(url)
	if err != nil {
		fmt.Fprintf(os.Stderr, "request error: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var pretty bytes.Buffer
	if json.Indent(&pretty, body, "", "  ") == nil {
		fmt.Println(pretty.String())
	} else {
		fmt.Println(string(body))
	}
}

func doPost(client *http.Client, url string, body any) {
	var bodyReader io.Reader
	if body != nil {
		data, _ := json.Marshal(body)
		bodyReader = bytes.NewReader(data)
	}
	resp, err := client.Post(url, "application/json", bodyReader)
	if err != nil {
		fmt.Fprintf(os.Stderr, "request error: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	var pretty bytes.Buffer
	if json.Indent(&pretty, respBody, "", "  ") == nil {
		fmt.Println(pretty.String())
	} else {
		fmt.Println(string(respBody))
	}
}

func listSubmissions(client *http.Client, server string) {
	doGet(client, server+"/api/v1/submissions")
}

func dispatchSubmission(client *http.Client, server, id string) {
	doPost(client, server+"/api/v1/submissions/"+id+"/dispatch", nil)
}

func getSummary(client *http.Client, server, id string) {
	doGet(client, server+"/api/v1/summaries/"+id)
}

func checkConsistency(client *http.Client, server, id string) {
	doPost(client, server+"/api/v1/summaries/"+id+"/check", nil)
}

func listAudit(client *http.Client, server string) {
	doGet(client, server+"/api/v1/audit")
}

func gatewayStatus(client *http.Client, server string) {
	doGet(client, server+"/api/v1/gateway/status")
}

func backlogStats(client *http.Client, server string) {
	doGet(client, server+"/api/v1/backlog")
}

func createSubmission(client *http.Client, server string, args []string) {
	sub := map[string]any{
		"enterprise_id":   "ent_demo",
		"enterprise_name": "Demo Rehabilitation Co.",
		"model_no":        "EXO-R1",
		"model_name":      "Rehab Exoskeleton V1",
		"category":        "exoskeleton",
		"risk_level":      2,
		"standard_codes":  []string{"GB-9706-1", "YY-0505"},
		"batch_no":        "B20260818-01",
	}
	doPost(client, server+"/api/v1/submissions", sub)
}
