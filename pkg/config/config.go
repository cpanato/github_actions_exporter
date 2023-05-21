package config

type Opts struct {
	MetricsPath          string
	ListenAddressMetrics string
	ListenAddressIngress string
	WebhookPath          string
	// GitHub webhook token.
	GitHubToken string
	// GitHub API token.
	GitHubAPIToken        string
	GitHubOrg             string
	GitHubUser            string
	GitHubRepos           []string
	BillingAPIPollSeconds int
}
