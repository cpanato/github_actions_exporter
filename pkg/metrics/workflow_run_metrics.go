package metrics

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/go-kit/log/level"
	"github.com/google/go-github/v50/github"
)

func (c *ExporterClient) StartWorkflowRuns(ctx context.Context) error {
	ticker := time.NewTicker(time.Duration(c.Opts.BillingAPIPollSeconds) * time.Second)
	go func() {
		for {
			select {
			case <-ticker.C:
				c.collectWorkflowRun(ctx)
			case <-ctx.Done():
				_ = level.Info(c.Logger).Log("msg", "stopped polling for workflow run metrics")
				return
			}
		}
	}()

	return nil
}

// getFieldValue return value from run element which corresponds to field
// func getFieldValue(repo string, run github.WorkflowRun, field string) string {
// 	switch field {
// 	case "repo":
// 		return repo
// 	case "id":
// 		return strconv.FormatInt(*run.ID, 10)
// 	case "node_id":
// 		return *run.NodeID
// 	case "head_branch":
// 		return *run.HeadBranch
// 	case "head_sha":
// 		return *run.HeadSHA
// 	case "run_number":
// 		return strconv.Itoa(*run.RunNumber)
// 	case "workflow_id":
// 		return strconv.FormatInt(*run.WorkflowID, 10)
// 	case "workflow":
// 		r, exist := workflows[repo]
// 		if !exist {
// 			log.Printf("Couldn't fetch repo '%s' from workflow cache.", repo)
// 			return "unknown"
// 		}
// 		w, exist := r[*run.WorkflowID]
// 		if !exist {
// 			log.Printf("Couldn't fetch repo '%s', workflow '%d' from workflow cache.", repo, *run.WorkflowID)
// 			return "unknown"
// 		}
// 		return *w.Name
// 	case "event":
// 		return *run.Event
// 	case "status":
// 		return *run.Status
// 	}
// 	log.Printf("Tried to fetch invalid field '%s'", field)
// 	return ""
// }

// func getRelevantFields(repo string, run *github.WorkflowRun) []string {
// 	relevantFields := strings.Split(config.WorkflowFields, ",")
// 	result := make([]string, len(relevantFields))
// 	for i, field := range relevantFields {
// 		result[i] = getFieldValue(repo, *run, field)
// 	}
// 	return result
// }

func (c *ExporterClient) getWorkflowRuns(ctx context.Context, owner string, repo string) []*github.WorkflowRun {
	opts := &github.ListWorkflowRunsOptions{
		ListOptions: github.ListOptions{
			PerPage: 200,
		},
		Created: ">=" + time.Now().Add(time.Duration(-12)*time.Hour).Format(time.RFC3339),
	}

	var runs []*github.WorkflowRun
	for {
		resp, rr, err := c.GHClient.Actions.ListRepositoryWorkflowRuns(ctx, owner, repo, opts)
		if rateErr, ok := err.(*github.RateLimitError); ok {
			log.Printf("ListRepositoryWorkflowRuns got rate limited. Sleeping until %s", rateErr.Rate.Reset.Time.String())
			time.Sleep(time.Until(rateErr.Rate.Reset.Time))
			continue
		} else if err != nil {
			log.Printf("ListRepositoryWorkflowRuns failed to retrieve workflow runs for repo %s/%s: %v", owner, repo, err)
			return runs
		}

		runs = append(runs, resp.WorkflowRuns...)
		if rr.NextPage == 0 {
			break
		}
		opts.Page = rr.NextPage
	}

	return runs
}

// func getRunUsage(owner string, repo string, runId int64) *github.WorkflowRunUsage {
// 	for {
// 		resp, _, err := client.Actions.GetWorkflowRunUsageByID(context.Background(), owner, repo, runId)
// 		if rl_err, ok := err.(*github.RateLimitError); ok {
// 			log.Printf("GetWorkflowRunUsageByID ratelimited. Pausing until %s", rl_err.Rate.Reset.Time.String())
// 			time.Sleep(time.Until(rl_err.Rate.Reset.Time))
// 			continue
// 		} else if err != nil {
// 			log.Printf("GetWorkflowRunUsageByID error for repo %s/%s and runId %d: %s", owner, repo, runId, err.Error())
// 			return nil
// 		}
// 		return resp
// 	}
// }

func (c *ExporterClient) collectWorkflowRun(ctx context.Context) {
	for _, repo := range c.Opts.GitHubRepos {
		r := strings.Split(repo, "/")
		runs := c.getWorkflowRuns(ctx, r[0], r[1])

		for _, run := range runs {
			workflowName := run.GetName()

			if run.GetStatus() == "completed" {
				seconds := run.UpdatedAt.Time.Sub(run.RunStartedAt.Time).Seconds()
				fmt.Println(run.GetRunNumber(), run.GetRunAttempt())

				c.PrometheusObserver.ObserveWorkflowRunDuration(
					r[0],
					r[1],
					run.GetStatus(),
					run.GetConclusion(),
					workflowName,
					seconds,
				)
				c.PrometheusObserver.CountWorkflowRunDuration(
					r[0],
					r[1],
					strconv.FormatInt(run.GetID(), 10),
					strconv.Itoa(run.GetRunAttempt()),
					strconv.Itoa(run.GetRunNumber()),
					run.GetStatus(),
					run.GetConclusion(),
					workflowName,
					seconds,
				)
			}
		}
	}

}
