package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/benitogf/detritus/internal/memory"
)

// The lesson gateway (`detritus --contribute`) is a VEHICLE, not a gate: it
// gathers EVERY local lesson and ships it into a single shared place (a
// `lessons/` dir in the target repo) via the normal PR flow. It applies no
// eligibility filter — Confirmed/Status/staleness ride along inside each file
// as data for a future central "janitor", never as a promotion gate. The human
// PR review is the only gate. The one content transform is transport hygiene:
// a conservative secret redactor masks obvious credential spans before writing,
// but never drops a lesson. See docs/flows/maintainer/contribute.md.

// contributeOptions are the parsed `--contribute` flags.
type contributeOptions struct {
	Repo   string // "owner/name"; "" ⇒ resolve the current repo via `gh repo view`
	Dir    string // directory in the target repo; default "lessons"
	DryRun bool   // print the plan, make NO git/gh mutations
}

// contribLessonDoc is one lesson bound for the target repo: its id, the
// repo-relative path it will be written to, and its redacted content.
type contribLessonDoc struct {
	ID      string
	RelPath string // e.g. "lessons/<id>.md"
	Content string // secrets redacted
}

// contributionPlan is the fully-resolved, side-effect-free description of what a
// contribution would do. Both the dry-run printer and the real executor consume
// it, so what `--dry-run` prints is exactly what the real run does.
type contributionPlan struct {
	Repo   string
	Branch string
	Dir    string
	Docs   []contribLessonDoc
	Title  string
	Body   string
}

// contribRun executes an external command (git or gh) in dir and returns its
// combined output. It is the single package-level seam tests replace with a
// fake so no real gh/git/network runs under test — the executor routes EVERY
// external command through it.
var contribRun = func(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	return cmd.CombinedOutput()
}

// contribNow is the clock used to name the contribution branch; a var so tests
// can pin a deterministic branch name.
var contribNow = time.Now

// repoNameRe validates a "owner/name" repo slug: one path segment, a slash, and
// a second segment, each restricted to the GitHub-safe alphabet. It blocks
// paths, flags, and shell-meaningful characters from reaching a gh/git call.
var repoNameRe = regexp.MustCompile(`^[A-Za-z0-9._-]+/[A-Za-z0-9._-]+$`)

// runContribute is the `detritus --contribute [--repo owner/name] [--dir path]
// [--dry-run]` handler: parse flags, build the (side-effect-free) plan, then
// either print it (--dry-run) or execute the vehicle (clone → branch → write →
// commit → push → PR).
func runContribute(cwd string, args []string) error {
	opts, err := parseContributeArgs(args)
	if err != nil {
		return err
	}
	ctx := context.Background()
	plan, err := buildContributionPlan(ctx, opts, cwd)
	if err != nil {
		return err
	}
	if len(plan.Docs) == 0 {
		fmt.Println("no local lessons to contribute (nothing under the memory lessons dir)")
		return nil
	}
	if opts.DryRun {
		printContributionPlan(os.Stdout, plan)
		return nil
	}
	return executeContribution(ctx, plan)
}

// parseContributeArgs parses the `--contribute` flags. Unknown flags are an
// error so a typo can't silently open a PR against the wrong place.
func parseContributeArgs(args []string) (contributeOptions, error) {
	opts := contributeOptions{Dir: "lessons"}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--dry-run":
			opts.DryRun = true
		case "--repo":
			if i+1 >= len(args) {
				return opts, fmt.Errorf("--repo requires a value (owner/name)")
			}
			i++
			opts.Repo = args[i]
		case "--dir":
			if i+1 >= len(args) {
				return opts, fmt.Errorf("--dir requires a value")
			}
			i++
			opts.Dir = strings.TrimSpace(args[i])
		default:
			return opts, fmt.Errorf("unknown --contribute flag: %s", args[i])
		}
	}
	if opts.Dir == "" {
		opts.Dir = "lessons"
	}
	// Validate before any git/gh call: guard against arg injection (leading '-'),
	// a malformed repo slug, and a --dir that escapes the temp clone.
	if strings.HasPrefix(opts.Repo, "-") {
		return opts, fmt.Errorf("--repo value must not begin with '-': %q", opts.Repo)
	}
	if opts.Repo != "" && !repoNameRe.MatchString(opts.Repo) {
		return opts, fmt.Errorf("--repo must be owner/name using letters, digits, '.', '_', '-': %q", opts.Repo)
	}
	if strings.HasPrefix(opts.Dir, "-") {
		return opts, fmt.Errorf("--dir value must not begin with '-': %q", opts.Dir)
	}
	if !filepath.IsLocal(opts.Dir) {
		return opts, fmt.Errorf("--dir must be a relative path inside the repo (no '..', no absolute paths): %q", opts.Dir)
	}
	return opts, nil
}

// buildContributionPlan resolves the target repo, gathers and redacts every
// local lesson, and composes the branch name + PR title/body. It makes NO git
// mutation: the only external call it may make is resolving the current repo's
// owner/name when --repo was omitted (via the contribRun seam).
func buildContributionPlan(ctx context.Context, opts contributeOptions, cwd string) (contributionPlan, error) {
	repo := opts.Repo
	if repo == "" {
		resolved, err := resolveContribRepo(ctx, cwd)
		if err != nil {
			return contributionPlan{}, fmt.Errorf("resolve target repo (pass --repo owner/name): %w", err)
		}
		repo = resolved
	}

	docs, err := gatherLessonDocs(opts.Dir)
	if err != nil {
		return contributionPlan{}, err
	}

	branch := fmt.Sprintf("lessons-contribution-%s", contribNow().UTC().Format("20060102-150405"))
	title := fmt.Sprintf("Contribute %d learned lesson(s)", len(docs))
	body := contributionBody(docs)
	return contributionPlan{
		Repo:   repo,
		Branch: branch,
		Dir:    opts.Dir,
		Docs:   docs,
		Title:  title,
		Body:   body,
	}, nil
}

// gatherLessonDocs reads EVERY local lesson (no filtering), redacts obvious
// secrets from each, and binds it to its repo-relative path under dir. The
// on-disk lesson format (frontmatter + bullets) is preserved verbatim except
// for masked secret spans, so the maturity metadata ships as data.
func gatherLessonDocs(dir string) ([]contribLessonDoc, error) {
	files, err := memory.AllLessonFiles()
	if err != nil {
		return nil, fmt.Errorf("list local lessons: %w", err)
	}
	var docs []contribLessonDoc
	for _, f := range files {
		raw, err := os.ReadFile(f.Path)
		if err != nil {
			return nil, fmt.Errorf("read lesson %s: %w", f.ID, err)
		}
		docs = append(docs, contribLessonDoc{
			ID:      f.ID,
			RelPath: path.Join(dir, f.ID+".md"),
			Content: redactSecrets(string(raw)),
		})
	}
	return docs, nil
}

// contributionBody renders the PR body: what the gateway is, that the PR review
// is the only gate, that redaction is transport hygiene, that the central
// curation janitor is a future step, and the list of shipped lesson ids.
func contributionBody(docs []contribLessonDoc) string {
	var b strings.Builder
	b.WriteString("Gathers locally-distilled lessons into the shared `lessons/` directory so they can be reviewed and curated centrally.\n\n")
	b.WriteString("- This is a transport vehicle, not a filter: **every** local lesson is shipped. Maturity fields (confirmed count, status, staleness) ride along inside each file as data — they are not promotion gates.\n")
	b.WriteString("- **This PR review is the only gate.** Reviewers accept, reject, or curate.\n")
	b.WriteString("- Secret masking is **best-effort** on common credential formats (keys, tokens, passwords, connection strings, PEM private-key blocks) — it is **not exhaustive**. Masking edits only the credential span; no lesson is dropped. **This PR review is the gate.**\n")
	b.WriteString("- Cross-user curation into `docs/` by a central janitor is a future step, out of scope here.\n\n")
	fmt.Fprintf(&b, "Lessons in this contribution (%d):\n", len(docs))
	for _, d := range docs {
		fmt.Fprintf(&b, "- `%s`\n", d.RelPath)
	}
	return b.String()
}

// printContributionPlan writes the dry-run summary: the resolved repo, the
// branch it would create, the lesson files it would write, and the PR title +
// body. It makes NO mutation — it only reports what a real run would do.
func printContributionPlan(w io.Writer, plan contributionPlan) {
	fmt.Fprintln(w, "--contribute (dry run) — no git/gh changes will be made")
	fmt.Fprintf(w, "Repo:   %s\n", plan.Repo)
	fmt.Fprintf(w, "Branch: %s\n", plan.Branch)
	fmt.Fprintf(w, "Files it would write (%d):\n", len(plan.Docs))
	for _, d := range plan.Docs {
		fmt.Fprintf(w, "  %s\n", d.RelPath)
	}
	fmt.Fprintf(w, "PR title: %s\n", plan.Title)
	fmt.Fprintln(w, "PR body:")
	for _, line := range strings.Split(strings.TrimRight(plan.Body, "\n"), "\n") {
		fmt.Fprintf(w, "  %s\n", line)
	}
}

// executeContribution runs the real vehicle: clone the target repo, create the
// branch, write every (redacted) lesson file, commit, push, and open the PR.
// Every external command goes through the contribRun seam; files are written
// with the os package into the clone dir. A clone dir is created under the OS
// temp dir and removed on return.
func executeContribution(ctx context.Context, plan contributionPlan) error {
	cloneDir, err := os.MkdirTemp("", "detritus-contribute-")
	if err != nil {
		return fmt.Errorf("create clone workdir: %w", err)
	}
	defer os.RemoveAll(cloneDir)

	if out, err := contribRun(ctx, "", "gh", "repo", "clone", plan.Repo, cloneDir); err != nil {
		return fmt.Errorf("clone %s: %w\n%s", plan.Repo, err, out)
	}
	if out, err := contribRun(ctx, cloneDir, "git", "checkout", "-b", plan.Branch); err != nil {
		return fmt.Errorf("create branch %s: %w\n%s", plan.Branch, err, out)
	}

	for _, d := range plan.Docs {
		dst := filepath.Join(cloneDir, filepath.FromSlash(d.RelPath))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return fmt.Errorf("mkdir for %s: %w", d.RelPath, err)
		}
		if err := os.WriteFile(dst, []byte(d.Content), 0o644); err != nil {
			return fmt.Errorf("write %s: %w", d.RelPath, err)
		}
	}

	if out, err := contribRun(ctx, cloneDir, "git", "add", plan.Dir); err != nil {
		return fmt.Errorf("git add %s: %w\n%s", plan.Dir, err, out)
	}
	if out, err := contribRun(ctx, cloneDir, "git", "commit", "-m", plan.Title); err != nil {
		return fmt.Errorf("git commit: %w\n%s", err, out)
	}
	if out, err := contribRun(ctx, cloneDir, "git", "push", "-u", "origin", plan.Branch); err != nil {
		return fmt.Errorf("git push: %w\n%s", err, out)
	}

	out, err := contribRun(ctx, cloneDir, "gh", "pr", "create",
		"--repo", plan.Repo, "--head", plan.Branch,
		"--title", plan.Title, "--body", plan.Body)
	if err != nil {
		createErr := fmt.Errorf("gh pr create: %w\n%s", err, out)
		if delOut, delErr := contribRun(ctx, cloneDir, "git", "push", "origin", "--delete", plan.Branch); delErr != nil {
			return fmt.Errorf("%w\nremote branch %q was pushed but PR creation failed, and deleting it also failed (%v) — delete it manually with: git push origin --delete %s\n%s", createErr, plan.Branch, delErr, plan.Branch, delOut)
		}
		return fmt.Errorf("%w\nremote branch %q was pushed but PR creation failed; the pushed branch was deleted", createErr, plan.Branch)
	}
	fmt.Printf("opened contribution PR: %s", string(out))
	if !strings.HasSuffix(string(out), "\n") {
		fmt.Println()
	}
	return nil
}

// resolveContribRepo returns the current repo's "owner/name" via `gh repo view`,
// routed through the contribRun seam so tests never touch a live gh.
func resolveContribRepo(ctx context.Context, cwd string) (string, error) {
	out, err := contribRun(ctx, cwd, "gh", "repo", "view", "--json", "nameWithOwner", "--jq", ".nameWithOwner")
	if err != nil {
		return "", err
	}
	nwo := strings.TrimSpace(string(out))
	if nwo == "" {
		return "", fmt.Errorf("empty repo name from gh")
	}
	return nwo, nil
}

// ---- Secret redaction (transport hygiene) ----------------------------------
//
// redactSecrets masks OBVIOUS credential spans and NOTHING else. It is
// deliberately conservative — it must never drop a lesson and must not
// over-redact ordinary prose or code. Detection is anchored on strong signals:
// PEM private-key blocks, well-known token prefixes, Authorization/Bearer
// headers, and `key: value` assignments whose key names a credential and whose
// value looks like one (≥8 non-space chars). A bare word like "password" or
// "token" in prose has no `=`/`:` + credential value, so it is left untouched.

const redactedMask = "[REDACTED]"

var (
	// PEM private-key blocks (any key type): mask the whole block.
	pemBlockRe = regexp.MustCompile(`(?s)-----BEGIN [A-Z0-9 ]*PRIVATE KEY-----.*?-----END [A-Z0-9 ]*PRIVATE KEY-----`)

	// Well-known credential token prefixes (OpenAI, Anthropic, Stripe secret/
	// restricted keys, GitHub, GitLab, Slack, AWS access key, Google API key).
	// Anchored on the vendor prefix so it cannot match arbitrary identifiers.
	// Stripe: only the SECRET (sk_live_) and RESTRICTED (rk_live_) forms are
	// masked; the PUBLISHABLE pk_ prefix is intentionally left alone.
	tokenPrefixRe = regexp.MustCompile(`\b(?:sk-ant-[A-Za-z0-9_\-]{8,}|sk-[A-Za-z0-9]{16,}|(?:sk|rk)_live_[A-Za-z0-9]{8,}|gh[pousr]_[A-Za-z0-9]{16,}|github_pat_[A-Za-z0-9_]{16,}|glpat-[A-Za-z0-9_\-]{16,}|xox[baprs]-[A-Za-z0-9\-]{8,}|AKIA[A-Z0-9]{16}|AIza[A-Za-z0-9_\-]{20,})\b`)

	// Connection-string password: scheme://user:PASSWORD@host — mask ONLY the
	// password between the ':' and the '@'. Requires the full user:pass@ shape
	// after '://' so a plain URL (no credentials) is never touched.
	connStringRe = regexp.MustCompile(`([A-Za-z][A-Za-z0-9+.\-]*://[^\s:/@]+:)([^\s/@]+)@`)

	// AWS secret access key: the labelled 40-char base64-ish value. The exact
	// aws_secret_access_key label + a fixed 40-char base64 alphabet makes false
	// positives negligible; the value is masked, the label preserved.
	awsSecretRe = regexp.MustCompile(`(?i)(aws_secret_access_key[\s:=]+["']?)([A-Za-z0-9/+]{40})`)

	// Authorization header / Bearer token: keep the "Authorization:"/"Bearer"
	// label, mask the credential.
	authHeaderRe = regexp.MustCompile(`(?i)(authorization\s*[:=]\s*)(?:bearer\s+)?[A-Za-z0-9._\-+/=]{8,}`)
	bearerRe     = regexp.MustCompile(`(?i)\bbearer\s+[A-Za-z0-9._\-+/=]{8,}`)

	// `credential-key: value` / `credential-key=value` assignments. The value
	// must be ≥8 non-space chars, so short prose values ("bucket", "enabled")
	// don't trip it. The key label and any quotes are preserved.
	assignSecretRe = regexp.MustCompile(`(?i)\b(password|passwd|passphrase|secret|client[_-]?secret|api[_-]?key|apikey|access[_-]?token|auth[_-]?token|refresh[_-]?token|token)(\s*[:=]\s*)(["']?)([^\s"'#]{8,})(["']?)`)

	// Bare JSON Web Token: anchored on the "eyJ" base64url header of the first
	// segment (a JSON "{" ), so ordinary hyphenated/dotted text (UUIDs, versions)
	// won't match. Catches JWTs that don't ride behind a Bearer label.
	jwtRe = regexp.MustCompile(`eyJ[A-Za-z0-9_\-]+\.[A-Za-z0-9_\-]+\.[A-Za-z0-9_\-]+`)
)

func redactSecrets(content string) string {
	content = pemBlockRe.ReplaceAllString(content, "-----BEGIN PRIVATE KEY----- [REDACTED] -----END PRIVATE KEY-----")
	content = tokenPrefixRe.ReplaceAllString(content, redactedMask)
	content = connStringRe.ReplaceAllString(content, "${1}***@")
	content = awsSecretRe.ReplaceAllString(content, "${1}"+redactedMask)
	content = authHeaderRe.ReplaceAllString(content, "${1}Bearer "+redactedMask)
	content = bearerRe.ReplaceAllString(content, "Bearer "+redactedMask)
	content = assignSecretRe.ReplaceAllString(content, "${1}${2}${3}"+redactedMask+"${5}")
	content = jwtRe.ReplaceAllString(content, redactedMask)
	return content
}
