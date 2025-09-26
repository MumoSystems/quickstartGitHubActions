package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"regexp"
	"strconv"

	"github.com/imroc/req/v3"
	"github.com/posener/goaction"
	"github.com/posener/goaction/actionutil"
)

type JiraIssue struct {
	Fields JiraIssueFields `json:"fields"`
	Update struct {
		IssueLinks []struct {
			Add struct {
				Values []struct {
					Type struct {
						Name string `json:"name"`
					} `json:"type"`
					OutwardIssues []struct {
						Key string `json:"key"`
					} `json:"outwardIssues"`
				} `json:"values"`
			} `json:"add"`
		} `json:"issuelinks"`
		ApproverGroups []struct {
			Add struct {
				Name    string `json:"name"`
				GroupID string `json:"groupId"`
				Self    string `json:"self"`
			} `json:"add"`
		} `json:"customfield_10080"`
	} `json:"update"`
}

type JiraIssueFields struct {
	Description struct {
		Type    string `json:"type"`
		Version int    `json:"version"`
		Content []struct {
			Type    string `json:"type"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"content"`
	} `json:"description"`
	GithubPRID string `json:"customfield_10533"`
	Type       struct {
		ID string `json:"id"`
	} `json:"issuetype"`
	Project struct {
		Key string `json:"key"`
	} `json:"project"`
	Summary string `json:"summary"`
}

type IssueResponse struct {
	ID   string `json:"id"`
	Key  string `json:"key"`
	Self string `json:"self"`
}

func getJIRAClient(jiraURL, jiraUser, jiraAPIToken string) *req.Client {
	client := req.C().
		SetBaseURL(jiraURL).
		SetCommonBasicAuth(jiraUser, jiraAPIToken).
		SetCommonHeader("Accept", "application/json").
		SetCommonHeader("Content-Type", "application/json")
	return client
}

func main() {
	ctx := context.Background()
	// Read environment variables
	jiraURL := os.Getenv("JIRA_URL")
	jiraAPIToken := os.Getenv("JIRA_API_TOKEN")
	jiraUser := os.Getenv("JIRA_USERNAME")
	jiraIssueDescription := os.Getenv("JIRA_ISSUE_DESCRIPTION")
	jiraIssueSummary := os.Getenv("JIRA_ISSUE_SUMMARY")
	jiraIssueTypeID := os.Getenv("JIRA_ISSUE_TYPE")
	jiraProject := os.Getenv("JIRA_PROJECT")
	token := os.Getenv("GITHUB_TOKEN")

	prEvent, err := goaction.GetPullRequest()
	if err != nil {
		log.Fatalf("Failed getting PR event information: %s", err)
	}
	// spew.Dump(prEvent)

	gh := actionutil.NewClientWithToken(ctx, token)
	re := regexp.MustCompile(`[A-Z]{2,}-\d+`)
	commits, _, err := gh.PullRequests.ListCommits(ctx, "mumosystems", "quickstartGitHubActions", *prEvent.Number, nil)
	if err != nil {
		log.Fatalf("Failed to fetch commits: %v", err)
	}

	// Collect unique issue keys
	issueKeys := make(map[string]bool)
	for _, commit := range commits {
		commitMessage := *commit.Commit.Message
		keys := re.FindAllString(commitMessage, -1)
		for _, key := range keys {
			issueKeys[key] = true
		}
	}

	// Create outward issues for each unique issue key found
	var outwardIssues []struct {
		Key string `json:"key"`
	}
	for key := range issueKeys {
		outwardIssues = append(outwardIssues, struct {
			Key string `json:"key"`
		}{
			Key: key,
		})
	}

	githubPRID := strconv.Itoa(*prEvent.Number)
	// Define approver groups
	approverGroups := []struct {
		Add struct {
			Name    string `json:"name"`
			GroupID string `json:"groupId"`
			Self    string `json:"self"`
		} `json:"add"`
	}{
		{
			Add: struct {
				Name    string `json:"name"`
				GroupID string `json:"groupId"`
				Self    string `json:"self"`
			}{
				Name:    "Change Management Board",
				GroupID: "5cd2dde1-4263-4b63-8e48-920e1e672e29",
				Self:    fmt.Sprintf("%srest/api/3/group?groupId=5cd2dde1-4263-4b63-8e48-920e1e672e29", jiraURL),
			},
		},
	}

	issue := buildIssuePayload(
		jiraIssueDescription,
		jiraIssueSummary,
		jiraIssueTypeID,
		jiraProject,
		githubPRID,
		outwardIssues,
		approverGroups,
	)

	// Initialize the Jira client
	client := getJIRAClient(jiraURL, jiraUser, jiraAPIToken)

	// Send the POST request to create the issue
	var result IssueResponse
	resp, err := client.R().
		SetContext(context.Background()).
		SetBody(&issue).
		SetSuccessResult(&result).
		Post("/rest/api/3/issue")
	if err != nil {
		log.Printf("Failed to create issue: %v", err)
		return
	}
	if resp.IsErrorState() {
		log.Printf("Error response: %s", resp.String())
		return
	}
	log.Printf("Issue created: %s", result.Self)
}

// buildIssuePayload builds the Jira issue payload.
func buildIssuePayload(
	jiraIssueDescription, jiraIssueSummary, jiraIssueTypeID, jiraProject, githubPRID string,
	outwardIssues []struct {
		Key string `json:"key"`
	},
	approverGroups []struct {
		Add struct {
			Name    string `json:"name"`
			GroupID string `json:"groupId"`
			Self    string `json:"self"`
		} `json:"add"`
	},
) JiraIssue {
	issue := JiraIssue{
		Fields: JiraIssueFields{
			Description: struct {
				Type    string `json:"type"`
				Version int    `json:"version"`
				Content []struct {
					Type    string `json:"type"`
					Content []struct {
						Type string `json:"type"`
						Text string `json:"text"`
					} `json:"content"`
				} `json:"content"`
			}{
				Type:    "doc",
				Version: 1,
				Content: []struct {
					Type    string `json:"type"`
					Content []struct {
						Type string `json:"type"`
						Text string `json:"text"`
					} `json:"content"`
				}{
					{
						Type: "paragraph",
						Content: []struct {
							Type string `json:"type"`
							Text string `json:"text"`
						}{
							{
								Type: "text",
								Text: jiraIssueDescription,
							},
						},
					},
				},
			},
			Type: struct {
				ID string `json:"id"`
			}{
				ID: jiraIssueTypeID,
			},
			Project: struct {
				Key string `json:"key"`
			}{
				Key: jiraProject,
			},
			Summary:    jiraIssueSummary,
			GithubPRID: githubPRID,
		},
	}

	// Add issue links if there are any issue keys
	if len(outwardIssues) > 0 {
		issue.Update.IssueLinks = []struct {
			Add struct {
				Values []struct {
					Type struct {
						Name string `json:"name"`
					} `json:"type"`
					OutwardIssues []struct {
						Key string `json:"key"`
					} `json:"outwardIssues"`
				} `json:"values"`
			} `json:"add"`
		}{
			{
				Add: struct {
					Values []struct {
						Type struct {
							Name string `json:"name"`
						} `json:"type"`
						OutwardIssues []struct {
							Key string `json:"key"`
						} `json:"outwardIssues"`
					} `json:"values"`
				}{
					Values: []struct {
						Type struct {
							Name string `json:"name"`
						} `json:"type"`
						OutwardIssues []struct {
							Key string `json:"key"`
						} `json:"outwardIssues"`
					}{
						{
							Type: struct {
								Name string `json:"name"`
							}{
								Name: "Blocks",
							},
							OutwardIssues: outwardIssues,
						},
					},
				},
			},
		}
	}

	// Add approver groups
	if len(approverGroups) > 0 {
		issue.Update.ApproverGroups = approverGroups
	}

	return issue
}
