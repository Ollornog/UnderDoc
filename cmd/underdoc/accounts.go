// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"
)

// Accounts on the command line (ADR-32). Everyone signs in through the gate; whoever runs the instance binds an account
// made before to its person in the gate, and in an emergency marks an administrator:
//
//	underdoc account list                      every account, with the lasting ID it is bound to
//	underdoc account bind <key|login> <id>     binds an account made before ADR-32 to the lasting ID of its person
//	underdoc admin grant <id>                  marks the account of that lasting ID as an administrator, made if need be
//	underdoc admin revoke <id>                 takes the mark back

// accountCommand tells whether the words after account or admin make a command.
func accountCommand(command string, rest []string) bool {
	switch {
	case command == "account" && len(rest) == 1 && rest[0] == "list":
	case command == "account" && len(rest) == 3 && rest[0] == "bind":
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
