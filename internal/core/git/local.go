package git

import (
	"repotory/internal/knowledge"
	"repotory/internal/stats"
)

// LocalGitProvider is the default Provider implementation that shells out to
// the locally installed `git` CLI and walks the working tree on disk. It is
// a thin, stateless adapter over the functions in internal/stats and
// internal/knowledge, keeping 100% of the existing behaviour (and tests).
type LocalGitProvider struct{}

// Compile-time check that LocalGitProvider implements Provider.
var _ Provider = (*LocalGitProvider)(nil)

// NewLocalGitProvider constructs a LocalGitProvider.
func NewLocalGitProvider() *LocalGitProvider {
	return &LocalGitProvider{}
}

func (p *LocalGitProvider) QueryStats(repoPath, date, author string) (*stats.Result, error) {
	return stats.QueryStats(repoPath, date, author)
}

func (p *LocalGitProvider) QueryStatsRange(repoPath, startDate, endDate, author string) ([]stats.DailyEntry, error) {
	return stats.QueryStatsRange(repoPath, startDate, endDate, author)
}

func (p *LocalGitProvider) GetRecentCommit(repoPaths []string, filterAuthor string) (*stats.RecentCommit, error) {
	return stats.GetRecentCommit(repoPaths, filterAuthor)
}

func (p *LocalGitProvider) GetRecentCommits(repoPaths []string, filterAuthor string, limit int) ([]stats.RecentCommit, error) {
	return stats.GetRecentCommits(repoPaths, filterAuthor, limit)
}

func (p *LocalGitProvider) MineKnowledge(repoPath string) (*knowledge.RepoKnowledge, error) {
	return knowledge.Mine(repoPath)
}

func (p *LocalGitProvider) ValidateDate(date string) error {
	return stats.ValidateDate(date)
}

func (p *LocalGitProvider) ValidateAuthor(author string) error {
	return stats.ValidateAuthor(author)
}

func (p *LocalGitProvider) GetTodayDate() string {
	return stats.GetTodayDate()
}

func (p *LocalGitProvider) GetYesterdayDate() string {
	return stats.GetYesterdayDate()
}

func (p *LocalGitProvider) IsWorkday(date string) bool {
	return stats.IsWorkday(date)
}
