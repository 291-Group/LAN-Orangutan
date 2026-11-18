package cli

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/291-Group/LAN-Orangutan/internal/notify"
	"github.com/291-Group/LAN-Orangutan/internal/storage"
	"github.com/291-Group/LAN-Orangutan/internal/types"
)

var webhookCmd = &cobra.Command{
	Use:   "webhook",
	Short: "List, add, edit, delete or test notification webhooks",
	Long: `With no subcommand (or with "list"), lists the webhooks, numbered for the
other commands. With --format json the output is the same object as
GET /api/webhooks.

A webhook is a URL that gets a JSON POST when a device event happens:
  new         a device is seen for the first time
  unexpected  a device not marked expected comes online
  missing     an expected device has not been seen for an hour
  moved       a device shows up at a new IP address

Telegram, Discord, Slack, ntfy and Home Assistant work by pasting their
webhook URL. URLs often contain a secret, so they are shown masked.`,
	Args: cobra.NoArgs,
	RunE: runWebhookList,
}

var (
	webhookFormat string
	webhookName   string
	webhookURL    string
	webhookEvents string
)

var webhookListCmd = &cobra.Command{
	Use:   "list",
	Short: "List the webhooks, numbered, with the last result",
	Args:  cobra.NoArgs,
	RunE:  runWebhookList,
}

var webhookAddCmd = &cobra.Command{
	Use:   "add <url>",
	Short: "Add a webhook (all four events unless --events is given)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := openStore()
		if err != nil {
			return err
		}
		events := notify.AllEvents
		if cmd.Flags().Changed("events") {
			events = strings.Split(webhookEvents, ",")
		}
		w, err := store.AddWebhook(webhookName, args[0], events)
		if err != nil {
			return err
		}
		fmt.Printf("Added webhook %s for %s\n", notify.MaskURL(w.URL), shortEvents(w.Events))
		fmt.Println(`Send a test with "orangutan webhook test <n>".`)
		return nil
	},
}

var webhookSetCmd = &cobra.Command{
	Use:   "set <n>",
	Short: "Change a webhook's name, URL or events",
	Long: `Change one or more of a webhook's fields, the same as editing it in the web
interface. Only the flags you pass are changed. Use "enable" and "disable" to
turn it on or off.`,
	Example: `  orangutan webhook set 1 --name "Phone"
  orangutan webhook set 1 --events new,missing
  orangutan webhook set 2 --url https://ntfy.sh/other-topic`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmd.Flags()
		var name, url *string
		var events *[]string
		if f.Changed("name") {
			name = &webhookName
		}
		if f.Changed("url") {
			url = &webhookURL
		}
		if f.Changed("events") {
			list := strings.Split(webhookEvents, ",")
			events = &list
		}
		if name == nil && url == nil && events == nil {
			return fmt.Errorf("nothing to change: pass at least one of --name, --url, --events")
		}
		store, w, err := webhookByNumber(args[0])
		if err != nil {
			return err
		}
		if err := store.UpdateWebhook(w.ID, name, url, events, nil); err != nil {
			return err
		}
		if updated, ok := store.GetWebhook(w.ID); ok {
			w = updated
		}
		fmt.Printf("Updated webhook %s for %s\n", webhookTitle(w), shortEvents(w.Events))
		return nil
	},
}

var webhookDeleteCmd = &cobra.Command{
	Use:     "delete <n>",
	Aliases: []string{"remove"},
	Short:   "Delete a webhook",
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		store, w, err := webhookByNumber(args[0])
		if err != nil {
			return err
		}
		if err := store.DeleteWebhook(w.ID); err != nil {
			return err
		}
		fmt.Printf("Deleted webhook %s\n", webhookTitle(w))
		return nil
	},
}

var webhookEnableCmd = &cobra.Command{
	Use:   "enable <n>",
	Short: "Turn a webhook on",
	Args:  cobra.ExactArgs(1),
	RunE:  func(cmd *cobra.Command, args []string) error { return setWebhookEnabled(args[0], true) },
}

var webhookDisableCmd = &cobra.Command{
	Use:   "disable <n>",
	Short: "Turn a webhook off without deleting it",
	Args:  cobra.ExactArgs(1),
	RunE:  func(cmd *cobra.Command, args []string) error { return setWebhookEnabled(args[0], false) },
}

var webhookTestCmd = &cobra.Command{
	Use:   "test <n>",
	Short: "Send a test message to a webhook and show the answer",
	Long: `Send a test message to a webhook and show the HTTP status it answered with.
A disabled webhook is tested all the same, since asking for a test is
explicit, but it still gets no real events until it is enabled.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		store, w, err := webhookByNumber(args[0])
		if err != nil {
			return err
		}
		if !w.Enabled {
			fmt.Fprintf(os.Stderr, "Note: webhook %s is disabled; sending the test anyway. Enable it with \"orangutan webhook enable %s\" to get real events.\n", webhookTitle(w), args[0])
		}
		res := notify.SendTest(context.Background(), store, w)
		if !res.Success {
			return fmt.Errorf("test to %s failed: %s", webhookTitle(w), res.Error)
		}
		if res.Body != "" {
			fmt.Printf("Test sent to %s (HTTP %d): %s\n", webhookTitle(w), res.Status, res.Body)
		} else {
			fmt.Printf("Test sent to %s (HTTP %d)\n", webhookTitle(w), res.Status)
		}
		return nil
	},
}

func init() {
	const eventsHelp = `comma-separated events to send: new, unexpected, missing, moved (the "device.new" form is accepted too)`
	webhookAddCmd.Flags().StringVar(&webhookName, "name", "", "a name to show in the list")
	webhookAddCmd.Flags().StringVar(&webhookEvents, "events", "new,unexpected,missing,moved", eventsHelp)
	webhookSetCmd.Flags().StringVar(&webhookName, "name", "", `a name to show in the list ("" to remove)`)
	webhookSetCmd.Flags().StringVar(&webhookURL, "url", "", "the URL to post to")
	webhookSetCmd.Flags().StringVar(&webhookEvents, "events", "", eventsHelp)
	for _, c := range []*cobra.Command{webhookCmd, webhookListCmd} {
		c.Flags().StringVar(&webhookFormat, "format", "table", "Output format (table, json)")
	}
	webhookCmd.AddCommand(webhookListCmd, webhookAddCmd, webhookSetCmd, webhookDeleteCmd, webhookEnableCmd, webhookDisableCmd, webhookTestCmd)
}

// webhookJSON is a webhook as GET /api/webhooks returns it, with the URL
// masked so a secret in it never ends up in a terminal log.
type webhookJSON struct {
	ID         string               `json:"id"`
	Name       string               `json:"name"`
	URL        string               `json:"url"`
	Events     []string             `json:"events"`
	Enabled    bool                 `json:"enabled"`
	LastResult *types.WebhookResult `json:"last_result,omitempty"`
}

func runWebhookList(cmd *cobra.Command, args []string) error {
	format, err := outputFormat(webhookFormat, "table", "json")
	if err != nil {
		return err
	}
	store, err := openStore()
	if err != nil {
		return err
	}
	hooks := store.GetWebhooks()
	if format == "json" {
		views := make([]webhookJSON, 0, len(hooks))
		for _, h := range hooks {
			views = append(views, webhookJSON{
				ID: h.ID, Name: h.Name, URL: notify.MaskURL(h.URL),
				Events: h.Events, Enabled: h.Enabled, LastResult: h.LastResult,
			})
		}
		return printJSON(map[string]any{
			"webhooks": views,
			"events":   notify.AllEvents,
		})
	}
	if len(hooks) == 0 {
		fmt.Println(`No webhooks. Add one with "orangutan webhook add <url>".`)
		return nil
	}
	rows := make([][]string, 0, len(hooks))
	for i, h := range hooks {
		rows = append(rows, []string{strconv.Itoa(i + 1), webhookTitle(h), shortEvents(h.Events), yesNo(h.Enabled), describeResult(h.LastResult)})
	}
	return printTable(os.Stdout, []string{"#", "WEBHOOK", "EVENTS", "ENABLED", "LAST RESULT"}, rows)
}

// webhookByNumber finds a webhook by its number in the list (1 is the first).
func webhookByNumber(arg string) (*storage.Storage, types.Webhook, error) {
	store, err := openStore()
	if err != nil {
		return nil, types.Webhook{}, err
	}
	hooks := store.GetWebhooks()
	n, err := strconv.Atoi(arg)
	if err != nil || n < 1 || n > len(hooks) {
		if len(hooks) == 0 {
			return nil, types.Webhook{}, fmt.Errorf("there are no webhooks")
		}
		return nil, types.Webhook{}, fmt.Errorf("no webhook %q: use a number from 1 to %d, as listed by \"orangutan webhook\"", arg, len(hooks))
	}
	return store, hooks[n-1], nil
}

func setWebhookEnabled(arg string, on bool) error {
	store, w, err := webhookByNumber(arg)
	if err != nil {
		return err
	}
	if err := store.UpdateWebhook(w.ID, nil, nil, nil, &on); err != nil {
		return err
	}
	state := "on"
	if !on {
		state = "off"
	}
	fmt.Printf("Webhook %s is %s\n", webhookTitle(w), state)
	return nil
}

// webhookTitle is the webhook's name when it has one, and its masked URL
// otherwise.
func webhookTitle(w types.Webhook) string {
	if w.Name != "" {
		return w.Name + " (" + notify.MaskURL(w.URL) + ")"
	}
	return notify.MaskURL(w.URL)
}

func shortEvents(events []string) string {
	short := make([]string, len(events))
	for i, e := range events {
		short[i] = notify.ShortEvent(e)
	}
	return strings.Join(short, ",")
}

func describeResult(r *types.WebhookResult) string {
	if r == nil {
		return "nothing sent yet"
	}
	when := formatTime(r.Time)
	if r.Success {
		return fmt.Sprintf("ok, HTTP %d at %s", r.Status, when)
	}
	return fmt.Sprintf("failed at %s: %s", when, r.Error)
}
