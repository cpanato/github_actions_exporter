package metrics

import (
	"context"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/go-kit/log/level"
	"github.com/google/go-github/v50/github"
)

func (c *ExporterClient) StartRunners(ctx context.Context) error {
	ticker := time.NewTicker(time.Duration(c.Opts.BillingAPIPollSeconds) * time.Second)
	go func() {
		for {
			select {
			case <-ticker.C:
				c.collectRunners(ctx)
			case <-ctx.Done():
				_ = level.Info(c.Logger).Log("msg", "stopped polling for org billing metrics")
				return
			}
		}
	}()

	return nil
}

func (c *ExporterClient) getRepoRunners(ctx context.Context, owner string, repo string) []*github.Runner {
	var runners []*github.Runner

	opts := &github.ListOptions{
		PerPage: 200,
	}

	for {
		resp, rr, err := c.GHClient.Actions.ListRunners(ctx, owner, repo, opts)
		if rateErr, ok := err.(*github.RateLimitError); ok {
			log.Printf("ListRunners got rate limited. Sleeping until %s", rateErr.Rate.Reset.Time.String())
			time.Sleep(time.Until(rateErr.Rate.Reset.Time))
			continue
		} else if err != nil {
			log.Printf("ListRunners failed to retrieve runners for repo %s: %v", repo, err)
			return nil
		}

		runners = append(runners, resp.Runners...)
		if rr.NextPage == 0 {
			break
		}
		opts.Page = rr.NextPage
	}

	return runners
}

func (c *ExporterClient) collectRunners(ctx context.Context) {
	for _, repo := range c.Opts.GitHubRepos {
		split := strings.Split(repo, "/")
		runners := c.getRepoRunners(ctx, split[0], split[1])
		for _, runner := range runners {
			if runner.GetStatus() == "online" {
				runnersGauge.WithLabelValues(repo, strconv.FormatInt(runner.GetID(), 10), runner.GetName(), runner.GetOS(), runner.GetStatus(), strconv.FormatBool(runner.GetBusy())).Set(1)
			} else {
				runnersGauge.WithLabelValues(repo, strconv.FormatInt(runner.GetID(), 10), runner.GetName(), runner.GetOS(), runner.GetStatus(), strconv.FormatBool(runner.GetBusy())).Set(0)
			}
		}
	}
}
