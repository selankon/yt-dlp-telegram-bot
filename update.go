package main

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/exp/slices"
)

const updateCmdTimeout = 10 * time.Minute

// maxUpdateOutput keeps the reply under Telegram's 4096 character limit.
const maxUpdateOutput = 3000

// restartExitCode is returned after a successful bot update. It's non-zero so
// systemd restarts the bot with Restart=on-failure too.
const restartExitCode = 3

var updateRunning atomic.Bool

// restartRequested is set after a successful bot update, so main exits with
// restartExitCode once the client stopped.
var restartRequested atomic.Bool

// botCtx is the telegram client's context and stopBot cancels it, both set in
// main.
var botCtx context.Context
var stopBot context.CancelFunc

// runUpdateCmd runs command with sh and returns the tail of its combined output.
func runUpdateCmd(ctx context.Context, command string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, updateCmdTimeout)
	defer cancel()

	var out bytes.Buffer
	cmd := NewCommand(ctx, "sh", "-c", command)
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	if ctx.Err() == context.DeadlineExceeded {
		err = fmt.Errorf("timed out after %v", updateCmdTimeout)
	}

	s := strings.TrimSpace(out.String())
	if r := []rune(s); len(r) > maxUpdateOutput {
		s = "…" + string(r[len(r)-maxUpdateOutput:])
	}
	return s, err
}

// handleCmdUpdate runs the configured update command for target ("ytdlp" or
// "bot"). Only admins can use it.
func handleCmdUpdate(ctx context.Context, msgCtx *MessageContext, target, command string) {
	if !slices.Contains(params.AdminUserIDs, msgCtx.FromUserID) {
		fmt.Println("  (update cmd from non-admin, ignoring)")
		return
	}
	if command == "" {
		_, _ = msgCtx.ReplyBuilder().Text(ctx, T("update_not_configured", nil))
		return
	}
	if !updateRunning.CompareAndSwap(false, true) {
		_, _ = msgCtx.ReplyBuilder().Text(ctx, T("update_already_running", nil))
		return
	}

	fmt.Println("  running", target, "update command:", command)
	_, _ = msgCtx.ReplyBuilder().Text(ctx, T("update_started", map[string]any{"Target": target}))

	// The command can take a while, so it runs outside the update handler.
	go func() {
		defer updateRunning.Store(false)
		ctx := botCtx

		output, err := runUpdateCmd(ctx, command)
		if err != nil {
			fmt.Println("  update error:", err)
			_, _ = msgCtx.ReplyBuilder().Text(ctx, T("update_failed", map[string]any{
				"Target": target,
				"Err":    err,
				"Output": output,
			}))
			return
		}
		fmt.Println("  update done")

		switch target {
		case "ytdlp":
			version, _, _ := ytdlpVersionCheckGetStr(ctx)
			_, _ = msgCtx.ReplyBuilder().Text(ctx, T("update_ytdlp_done", map[string]any{
				"Output":  output,
				"Version": version,
			}))
		case "bot":
			_, _ = msgCtx.ReplyBuilder().Text(ctx, T("update_bot_done", map[string]any{"Output": output}))
			fmt.Println("bot updated, exiting to restart")
			restartRequested.Store(true)
			stopBot()
		}
	}()
}
