package main

import (
	"context"
	"net/http"

	"github.com/fiatjaf/etleneum/types"
	"github.com/google/go-github/github"
	"github.com/jmoiron/sqlx"
)

type ghRoundTripper struct{}

func (_ ghRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	r.Header.Set("Authorization", "Token "+s.GitHubToken)
	return http.DefaultTransport.RoundTrip(r)
}

var ghHttpClient = &http.Client{
	Transport: ghRoundTripper{},
}
var gh = github.NewClient(ghHttpClient)

func handleGitHubWebhook(w http.ResponseWriter, r *http.Request) {
	if s.GitHubToken != "" {
		go getGitHubChanges()
	}
	w.WriteHeader(200)
}

func getGitHubChanges() {
	tx, _ := pg.Beginx()
	defer tx.Rollback()

	var head string
	err := tx.Get(&head, `
      SELECT (jsonb_build_object('v', v)->>'v')::text
      FROM kv
      WHERE k = 'github_head'
    `)
	if err != nil {
		log.Error().Err(err).Msg("fetching git head from database")
		return
	}

	log.Debug().Str("since", head).Msg("getting github changes")

	prev, _, err := gh.Git.GetTree(context.Background(),
		s.GitHubRepoOwner, s.GitHubRepoName,
		head, false)
	if err != nil {
		log.Error().Err(err).Msg("fetching head tree")
		return
	}

	ref, _, err := gh.Git.GetRef(context.Background(),
		s.GitHubRepoOwner, s.GitHubRepoName,
		"refs/heads/master",
	)
	if err != nil {
		log.Error().Err(err).Msg("fetching next ref")
		return
	}

	nextSHA := *(*ref.Object).SHA
	if nextSHA == head {
		log.Info().Msg("already at the latest github state")
		return
	}

	next, _, err := gh.Git.GetTree(context.Background(),
		s.GitHubRepoOwner, s.GitHubRepoName,
		nextSHA, false)
	if err != nil {
		log.Error().Err(err).Msg("fetching next tree")
		return
	}

	prevEntries := make(map[string]string)
	for _, entry := range prev.Entries {
		if *entry.Type == "tree" {
			prevEntries[*entry.Path] = *entry.SHA
		}
	}

	for _, entry := range next.Entries {
		if sha, ok := prevEntries[*entry.Path]; ok && sha != *entry.SHA {
			// different
			err := updateContractFromTree(tx, *entry.Path, *entry.SHA)
			if err != nil {
				return
			}
		}
	}

	_, err = tx.Exec(`
      UPDATE kv
      SET v = to_jsonb($1::text)
      WHERE k = 'github_head'
    `, nextSHA)
	if err != nil {
		log.Error().Err(err).Msg("updating git head on database")
		return
	}

	tx.Commit()
}

func updateContractFromTree(tx *sqlx.Tx, contractId, sha string) error {
	tree, _, err := gh.Git.GetTree(context.Background(),
		s.GitHubRepoOwner, s.GitHubRepoName,
		sha, false)
	if err != nil {
		log.Warn().Err(err).Str("ctid", contractId).Msg("fetching subdir tree")
		return err
	}

	var name string
	var code string
	var readme string

	for _, entry := range tree.Entries {
		bvalue, _, err := gh.Git.GetBlobRaw(context.Background(),
			s.GitHubRepoOwner, s.GitHubRepoName,
			*entry.SHA,
		)
		if err != nil {
			log.Warn().Err(err).Str("ctid", contractId).Str("path", *entry.Path).
				Msg("fetching blob")
			return err
		}

		switch *entry.Path {
		case "name.txt":
			name = string(bvalue)
		case "contract.lua":
			code = string(bvalue)
		case "README.md":
			readme = string(bvalue)
		}
	}

	_, err = tx.Exec(`
      UPDATE contracts SET name = $1, readme = $2, code = $3
      WHERE id = $4
    `, name, readme, code, contractId)
	if err != nil {
		log.Warn().Err(err).Str("ctid", contractId).
			Str("name", name).
			Str("code", code).
			Str("readme", readme).
			Msg("updating contract on database")
		return err
	}

	return nil
}

func saveContractOnGitHub(ct *types.Contract) {
	if s.GitHubToken == "" {
		return
	}

	ref, _, err := gh.Git.GetRef(context.Background(),
		s.GitHubRepoOwner, s.GitHubRepoName,
		"refs/heads/master",
	)
	if err != nil {
		log.Error().Err(err).Msg("getting current ref before updating github")
		return
	}

	commit, _, err := gh.Git.GetCommit(context.Background(),
		s.GitHubRepoOwner, s.GitHubRepoName,
		*(*ref.Object).SHA,
	)
	if err != nil {
		log.Error().Err(err).Msg("getting current commit before updating github")
		return
	}

	cttree, _, err := gh.Git.CreateTree(context.Background(),
		s.GitHubRepoOwner, s.GitHubRepoName,
		"",
		[]github.TreeEntry{
			{
				Mode:    github.String("100644"),
				Type:    github.String("blob"),
				Path:    github.String("name.txt"),
				Content: github.String(ct.Name),
			},
			{
				Mode:    github.String("100644"),
				Type:    github.String("blob"),
				Path:    github.String("README.md"),
				Content: github.String(ct.Readme),
			},
			{
				Mode:    github.String("100644"),
				Type:    github.String("blob"),
				Path:    github.String("contract.lua"),
				Content: github.String(ct.Code),
			},
		},
	)
	if err != nil {
		log.Error().Err(err).Str("id", ct.Id).Msg("creating new tree on github")
		return
	}

	tree, _, err := gh.Git.CreateTree(context.Background(),
		s.GitHubRepoOwner, s.GitHubRepoName,
		*(*commit.Tree).SHA,
		[]github.TreeEntry{
			{
				Mode: github.String("040000"),
				Type: github.String("tree"),
				Path: github.String(ct.Id),
				SHA:  github.String(*cttree.SHA),
			},
		},
	)
	if err != nil {
		log.Error().Err(err).Str("id", ct.Id).Msg("creating new tree on github")
		return
	}

	newcommit, _, err := gh.Git.CreateCommit(context.Background(),
		s.GitHubRepoOwner, s.GitHubRepoName,
		&github.Commit{
			Parents: []github.Commit{*commit},
			Tree:    tree,
			Message: github.String("created contract '" + ct.Name + "' (" + ct.Id + ")"),
		},
	)
	if err != nil {
		log.Error().Err(err).Str("id", ct.Id).Msg("creating new commit on github")
		return
	}

	_, _, err = gh.Git.UpdateRef(context.Background(),
		s.GitHubRepoOwner, s.GitHubRepoName,
		&github.Reference{
			Ref: github.String("refs/heads/master"),
			Object: &github.GitObject{
				SHA: newcommit.SHA,
			},
		},
		false,
	)
	if err != nil {
		log.Error().Err(err).Str("id", ct.Id).Msg("updating tree on github")
		return
	}

	_, err = pg.Exec(`
      UPDATE kv
      SET v = to_jsonb($1::text)
      WHERE k = 'github_head'
    `, *newcommit.SHA)
	if err != nil {
		log.Error().Err(err).Msg("updating git head on database")
		return
	}

	log.Debug().Str("id", ct.Id).Msg("created contract on github")
}
