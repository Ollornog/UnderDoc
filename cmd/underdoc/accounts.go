// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"text/tabwriter"

	"github.com/Ollornog/UnderDoc/internal/db"
	"github.com/Ollornog/UnderDoc/internal/gateapi"
)

// Accounts on the command line (ADR-32). Everyone signs in through the gate; whoever runs the instance binds an account
// made before to its person in the gate, and in an emergency marks an administrator:
//
//	underdoc account list                      every account, with the lasting ID it is bound to
//	underdoc account bind <key|login> <id>     binds an account made before ADR-32 to the lasting ID of its person
//	underdoc account add <id> <login> <name>   an account bound to a lasting ID of the gate before its first sign-in,
//	                                           without an administrator's mark, for an import (T-33)
//	underdoc account adopt [--dry-run]         an account for every account the gate's account API shows UnderDoc,
//	                                           bound by its lasting ID: those the gate's operator handed over (T-33)
//	underdoc admin grant <id>                  marks the account of that lasting ID as an administrator, made if need be
//	underdoc admin revoke <id>                 takes the mark back

// accountCommand tells whether the words after account or admin make a command.
func accountCommand(command string, rest []string) bool {
	switch {
	case command == "account" && len(rest) == 1 && rest[0] == "list":
	case command == "account" && len(rest) == 3 && rest[0] == "bind":
	case command == "account" && len(rest) == 4 && rest[0] == "add":
	case command == "account" && len(rest) >= 1 && len(rest) <= 2 && rest[0] == "adopt" && (len(rest) == 1 || rest[1] == "--dry-run"):
	case command == "admin" && len(rest) == 2 && (rest[0] == "grant" || rest[0] == "revoke"):
	default:
		return false
	}
	return true
}

func accountsCommand(ctx context.Context, command string, rest []string, stdout io.Writer) error {
	database, err := openDatabase(ctx)
	if err != nil {
		return err
	}
	defer database.Close()
	switch {
	case command == "account" && rest[0] == "list":
		all, err := database.Accounts(ctx)
		if err != nil {
			return err
		}
		out := tabwriter.NewWriter(stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(out, "KEY\tLOGIN\tNAME\tGATE ID\tSOURCE\tMARKED ADMIN")
		for _, a := range all {
			bound := a.RemoteID
			if bound == "" {
				bound = "– not bound"
			}
			marked := ""
			if a.Admin {
				marked = "✓"
			}
			fmt.Fprintf(out, "%s\t%s\t%s\t%s\t%s\t%s\n", a.ID, a.Login, a.Name, bound, a.Source, marked)
		}
		return out.Flush()
	case command == "account" && rest[0] == "add":
		a, created, err := database.AddAccount(ctx, rest[1], rest[2], rest[3], "")
		if err != nil {
			return fmt.Errorf("add %s: %w", rest[1], err)
		}
		if !created {
			fmt.Fprintf(stdout, "the lasting ID %s of the gate is bound to account %s (%s) already; nothing changed\n", a.RemoteID, a.ID, a.Login)
			return nil
		}
		_ = database.Log(ctx, "", db.LogAccount, "", fmt.Sprintf("%s (gate %s) added on the command line", a.Login, a.RemoteID))
		fmt.Fprintf(stdout, "account %s (%s) is made, bound to the lasting ID %s of the gate, without an administrator's mark\n", a.ID, a.Login, a.RemoteID)
	case command == "account" && rest[0] == "adopt":
		return adoptAccounts(ctx, database, len(rest) == 2, stdout)
	case command == "account":
		a, err := database.Bind(ctx, rest[1], rest[2])
		if err != nil {
			return fmt.Errorf("bind %s: %w", rest[1], err)
		}
		fmt.Fprintf(stdout, "account %s (%s) is bound to the lasting ID %s of the gate\n", a.ID, a.Login, a.RemoteID)
	default:
		grant := rest[0] == "grant"
		a, err := database.MarkAdmin(ctx, rest[1], grant)
		if err != nil {
			return fmt.Errorf("admin %s %s: %w", rest[0], rest[1], err)
		}
		if grant {
			fmt.Fprintf(stdout, "account %s (%s), lasting ID %s, is marked as an administrator; take the mark back with underdoc admin revoke once the gate gives the role again\n", a.ID, a.Login, a.RemoteID)
		} else {
			fmt.Fprintf(stdout, "account %s (%s), lasting ID %s, is no longer marked as an administrator\n", a.ID, a.Login, a.RemoteID)
		}
	}
	return nil
}

// adoptAccounts makes an account of UnderDoc for every account the gate's account API shows it (T-33): those UnderDoc
// invited, and those the gate's operator handed to UnderDoc (tinysesam accounts adopt). Each is bound by its lasting ID,
// never by a name; one bound already stays as it is. dry tells what would be made without making it.
func adoptAccounts(ctx context.Context, database *db.DB, dry bool, stdout io.Writer) error {
	url, keyFile := os.Getenv("UNDERDOC_GATE_URL"), os.Getenv("UNDERDOC_GATE_SERVICE_KEY_FILE")
	if url == "" || keyFile == "" {
		return errors.New("adopt needs UNDERDOC_GATE_URL and UNDERDOC_GATE_SERVICE_KEY_FILE")
	}
	client, err := gateapi.New(gateapi.Config{URL: url, KeyFile: keyFile})
	if err != nil {
		return err
	}
	made, kept := 0, 0
	for after := ""; ; {
		page, next, err := client.List(ctx, after)
		if err != nil {
			return fmt.Errorf("the gate's account API: %w", err)
		}
		for _, a := range page {
			name := a.DisplayName
			if name == "" {
				name = a.Username
			}
			if existing, err := database.AccountByRemoteID(ctx, a.ID); err == nil {
				kept++
				fmt.Fprintf(stdout, "kept   %s (gate %s): account %s\n", a.Username, a.ID, existing.ID)
				continue
			} else if !errors.Is(err, db.ErrNotFound) {
				return err
			}
			if dry {
				made++
				fmt.Fprintf(stdout, "would  %s (gate %s), %s\n", a.Username, a.ID, name)
				continue
			}
			account, _, err := database.AddAccount(ctx, a.ID, a.Username, name, a.Email)
			if err != nil {
				return fmt.Errorf("adopt %s (gate %s): %w", a.Username, a.ID, err)
			}
			_ = database.Log(ctx, "", db.LogAccount, "", fmt.Sprintf("%s (gate %s) adopted from the gate's account API", a.Username, a.ID))
			made++
			fmt.Fprintf(stdout, "made   %s (gate %s): account %s\n", a.Username, a.ID, account.ID)
		}
		if next == "" {
			break
		}
		after = next
	}
	verb := "made"
	if dry {
		verb = "would be made"
	}
	fmt.Fprintf(stdout, "%d %s, %d bound already\n", made, verb, kept)
	return nil
}
