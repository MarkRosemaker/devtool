package ghrepo

import "github.com/google/go-github/v80/github"

// Option is a functional option for configuring Repository creation/opening
type Option func(*repoConfig)

// repoConfig holds the configuration (internal)
type repoConfig struct {
	baseDir        string
	mkdirAll       bool
	cloneGit       bool
	initGit        bool
	createRemote   bool
	onGithub       *github.Repository
	createOnGitHub bool
	ownerIsOrg     bool

	// token          string
	// private        bool
	// description    string
	// defaultBranch  string
	// initialCommit  string
	// autoStage      bool
	// easily extendable in the future
}

// WithBaseDir sets a custom base directory for the repository.
// If not set, uses the current directory.
func WithBaseDir(baseDir string) Option {
	return func(o *repoConfig) { o.baseDir = baseDir }
}

// WithGithubRepo sets the GitHub repository to link to, so that no fetching/creation is needed.
func WithGithubRepo(r *github.Repository) Option {
	return func(o *repoConfig) { o.onGithub = r }
}

// MakeDirAll instructs the initializer to create the repository directory, if it does not exists.
func MakeDirAll(o *repoConfig) { o.mkdirAll = true }

// CloneGit instructs the initializer to clone git in the repository directory, if it does not exist.
func CloneGit(o *repoConfig) { o.cloneGit = true }

// InitGit instructs the initializer to initialize git in the repository directory, if it was not initialized.
func InitGit(o *repoConfig) { o.initGit = true }

// CreateRemote instructs the initializer to create a remote, if it does not exists.
func CreateRemote(o *repoConfig) { o.createRemote = true }

// CreateOnGitHub instructs the initializer to create the repository on GitHub, if it does not exists.
func CreateOnGitHub(o *repoConfig) { o.createOnGitHub = true }

// OwnerIsOrg clarifies that the given owner is not the user, but an organization on GitHub.
func OwnerIsOrg(o *repoConfig) { o.ownerIsOrg = true }

// ghrepo.CreateOnGitHub(),
// ghrepo.WithDescription("My awesome project"),
//     ghrepo.WithInitialCommit("Initial commit"),
//     ghrepo.AutoStageAll(),
