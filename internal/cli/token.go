package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/291-Group/LAN-Orangutan/internal/auth"
)

var tokenFormat string

var tokenCmd = &cobra.Command{
	Use:   "token",
	Short: "Create or revoke the API token",
	Long: `With no subcommand, says whether an API token exists. With --format json the
output is the same object as GET /api/token.

The token lets scripts and other programs use the API. It is shown once, when
it is created, and only a hash of it is stored, so a lost token is replaced,
not looked up. A running server picks up a new or revoked token immediately.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		format, err := outputFormat(tokenFormat, "table", "json")
		if err != nil {
			return err
		}
		exists := auth.LoadAPITokenHash(cfg.APITokenFile()) != ""
		if format == "json" {
			return printJSON(map[string]bool{"exists": exists})
		}
		if exists {
			fmt.Println(`An API token exists. "orangutan token create" replaces it, "orangutan token revoke" removes it.`)
		} else {
			fmt.Println(`No API token. Create one with "orangutan token create".`)
		}
		return nil
	},
}

var tokenCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create an API token, replacing any existing one",
	Long: `Create an API token, replacing any existing one. Only the token itself goes
to standard output, so TOKEN=$(orangutan token create) captures just the token.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		replacing := auth.LoadAPITokenHash(cfg.APITokenFile()) != ""
		token, hash, err := auth.NewAPIToken()
		if err != nil {
			return err
		}
		if err := auth.SaveAPITokenHash(cfg.APITokenFile(), hash); err != nil {
			return err
		}
		fmt.Println(token)
		// Everything that is not the token goes to stderr so a script can
		// capture stdout and get the token alone.
		fmt.Fprintln(os.Stderr)
		if replacing {
			fmt.Fprintln(os.Stderr, "The previous token no longer works.")
		}
		fmt.Fprintln(os.Stderr, "This is the only time the token is shown. Use it as:")
		fmt.Fprintln(os.Stderr, "  Authorization: Bearer <token>")
		return nil
	},
}

var tokenRevokeCmd = &cobra.Command{
	Use:   "revoke",
	Short: "Revoke the API token",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if auth.LoadAPITokenHash(cfg.APITokenFile()) == "" {
			fmt.Println("There is no API token to revoke.")
			return nil
		}
		if err := auth.SaveAPITokenHash(cfg.APITokenFile(), ""); err != nil {
			return err
		}
		fmt.Println("API token revoked. It stops working immediately.")
		return nil
	},
}

func init() {
	tokenCmd.Flags().StringVar(&tokenFormat, "format", "table", "Output format (table, json)")
	tokenCmd.AddCommand(tokenCreateCmd, tokenRevokeCmd)
}
