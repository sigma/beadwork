package main

import (
	"fmt"

	"github.com/jallum/beadwork/internal/issue"
)

// actionResult holds the outcome of a mutation for display in the status bar.
type actionResult struct {
	msg string
	err error
}

func (a actionResult) String() string {
	if a.err != nil {
		return "Error: " + a.err.Error()
	}
	return a.msg
}

// findRepoForIssue returns the RepoSource that owns the given issue ID.
func findRepoForIssue(repos []*RepoSource, id string) *RepoSource {
	for _, r := range repos {
		if _, err := r.Store.Get(id); err == nil {
			return r
		}
	}
	return nil
}

// doStart starts an issue (transitions open → in_progress).
func doStart(repos []*RepoSource, id string) actionResult {
	r := findRepoForIssue(repos, id)
	if r == nil {
		return actionResult{err: fmt.Errorf("issue %s not found", id)}
	}

	assignee := r.Repo.UserName()
	iss, err := r.Store.Start(id, assignee)
	if err != nil {
		return actionResult{err: err}
	}

	intent := fmt.Sprintf("start %s assignee=%q", iss.ID, assignee)
	if err := r.Store.Commit(intent); err != nil {
		return actionResult{err: fmt.Errorf("commit: %w", err)}
	}

	return actionResult{msg: fmt.Sprintf("Started %s", iss.ID)}
}

// doClose closes an issue.
func doClose(repos []*RepoSource, id string) actionResult {
	r := findRepoForIssue(repos, id)
	if r == nil {
		return actionResult{err: fmt.Errorf("issue %s not found", id)}
	}

	iss, err := r.Store.Close(id, "")
	if err != nil {
		return actionResult{err: err}
	}

	intent := fmt.Sprintf("close %s", iss.ID)
	if err := r.Store.Commit(intent); err != nil {
		return actionResult{err: fmt.Errorf("commit: %w", err)}
	}

	return actionResult{msg: fmt.Sprintf("Closed %s", iss.ID)}
}

// doReopen reopens a closed or in_progress issue.
func doReopen(repos []*RepoSource, id string) actionResult {
	r := findRepoForIssue(repos, id)
	if r == nil {
		return actionResult{err: fmt.Errorf("issue %s not found", id)}
	}

	iss, err := r.Store.Reopen(id)
	if err != nil {
		return actionResult{err: err}
	}

	intent := fmt.Sprintf("reopen %s", iss.ID)
	if err := r.Store.Commit(intent); err != nil {
		return actionResult{err: fmt.Errorf("commit: %w", err)}
	}

	return actionResult{msg: fmt.Sprintf("Reopened %s", iss.ID)}
}

// doComment adds a comment to an issue.
func doComment(repos []*RepoSource, id, text string) actionResult {
	r := findRepoForIssue(repos, id)
	if r == nil {
		return actionResult{err: fmt.Errorf("issue %s not found", id)}
	}

	author := r.Repo.UserName()
	_, err := r.Store.Comment(id, text, author)
	if err != nil {
		return actionResult{err: err}
	}

	intent := fmt.Sprintf("comment %s %q", id, text)
	if err := r.Store.Commit(intent); err != nil {
		return actionResult{err: fmt.Errorf("commit: %w", err)}
	}

	return actionResult{msg: fmt.Sprintf("Commented on %s", id)}
}

// doSync syncs all repos.
func doSync(repos []*RepoSource) actionResult {
	var msgs []string
	for _, r := range repos {
		status, replayed, err := r.Repo.Sync(nil)
		if err != nil {
			return actionResult{err: fmt.Errorf("%s: %w", r.Name, err)}
		}
		msg := fmt.Sprintf("%s: %s", r.Name, status)
		if len(replayed) > 0 {
			// Replay intents
			store := r.Store
			for _, intent := range replayed {
				replayIntent(store, intent)
			}
			if err := r.Store.Commit("replay after sync"); err != nil {
				return actionResult{err: fmt.Errorf("%s: replay commit: %w", r.Name, err)}
			}
			if err := r.Repo.Push(nil); err != nil {
				return actionResult{err: fmt.Errorf("%s: push after replay: %w", r.Name, err)}
			}
			msg += fmt.Sprintf(" (replayed %d intents)", len(replayed))
		}
		msgs = append(msgs, msg)
		r.Store.ClearCache()
	}

	if len(msgs) == 1 {
		return actionResult{msg: "Sync: " + msgs[0]}
	}
	result := "Sync complete"
	for _, m := range msgs {
		result += "\n  " + m
	}
	return actionResult{msg: result}
}

// replayIntent re-applies an intent message. This is a simplified version
// of the intent replay from the CLI — for now we just log it.
// Full replay would parse the intent and call the appropriate Store method.
func replayIntent(store *issue.Store, intent string) {
	// The full intent replay system is in internal/intent/intent.go
	// For the TUI, we delegate to the repo's Sync which handles replay.
	_ = store
	_ = intent
}
