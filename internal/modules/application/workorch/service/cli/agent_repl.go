package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/muidea/workorch/internal/modules/application/workorch/biz/agent"
)

func startAgentREPL() {
	printBanner()
	rl := newReadline()
	var currentWS string
	var lastGoal string

	ctx := context.Background()
	wsID, err := agent.CreateWorkspace(ctx, "agent-session", "interactive agent session")
	if err == nil {
		currentWS = wsID
		fmt.Fprintf(os.Stderr, "\r  workspace: %s\r\n", styleDim.Render(wsID))
	}

	for {
		fmt.Fprint(os.Stderr, "\r\n")
		prompt := fmt.Sprintf("%s%s", styleDim.Render(currentWS[:min(8, len(currentWS))]+" "), styleInfo.Render("▸ "))
		input, err := rl.ReadLine(prompt)
		if err != nil || input == "" {
			continue
		}
		input = strings.TrimSpace(input)
		if input == "" {
			continue
		}

		switch {
		case input == "/quit" || input == "/exit" || input == "/q":
			fmt.Fprintf(os.Stderr, "\rbye.\r\n")
			return
		case input == "/help" || input == "/?":
			printAgentHelp()
		case input == "/new":
			currentWS = ""
			lastGoal = ""
			wsID, err := agent.CreateWorkspace(ctx, "agent-session", "interactive agent session")
			if err == nil {
				currentWS = wsID
				fmt.Fprintf(os.Stderr, "\r  workspace: %s\r\n", styleDim.Render(wsID))
			} else {
				fmt.Fprintf(os.Stderr, "\r  Started new session.\r\n")
			}
		case input == "/retry":
			if lastGoal == "" {
				fmt.Fprintf(os.Stderr, "\r  Nothing to retry.\r\n")
			} else {
				currentWS = runAgentREPL(lastGoal, currentWS)
			}
		case strings.HasPrefix(input, "/"):
			parts := strings.Fields(input)
			fmt.Fprintf(os.Stderr, "\runknown: %s (type /help)\r\n", parts[0])
		default:
			lastGoal = input
			currentWS = runAgentREPL(input, currentWS)
		}
	}
}

func printBanner() {
	fmt.Fprint(os.Stderr, "\r  WorkOrch Agent\r\n")
	fmt.Fprint(os.Stderr, "\r  Type your goal. /new to start fresh, /quit to exit.\r\n")
}

func printAgentHelp() {
	fmt.Fprint(os.Stderr, "\r\n")
	fmt.Fprint(os.Stderr, "\r  Type any text to start the AI agent on that goal.\r\n")
	fmt.Fprint(os.Stderr, "\r    /new           Start a new workspace\r\n")
	fmt.Fprint(os.Stderr, "\r    /retry         Retry the last failed goal\r\n")
	fmt.Fprint(os.Stderr, "\r    /help, /?      Show this help\r\n")
	fmt.Fprint(os.Stderr, "\r    /quit, /q      Exit\r\n")
	fmt.Fprint(os.Stderr, "\r\n")
}

func runAgentREPL(goalText string, currentWS string) string {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	fmt.Fprintf(os.Stderr, "\r→ %s\r\n", styleDim.Render(goalText))

	a := agent.New()
	defer a.Close()

	startedAt := time.Now()
	tickCount := 0

	go func() {
		_ = a.Run(ctx, currentWS, goalText, "")
	}()

	for evt := range a.Events() {
		switch evt.Type {
		case agent.EventProgress:
			tickCount++
			if tickCount%6 == 0 {
				fmt.Fprint(os.Stderr, styleDim.Render("."))
			}

		case agent.EventRunCreated:
			fmt.Fprintf(os.Stderr, "\r  run: %s\r\n", styleDim.Render(evt.Payload.(agent.RunCreatedInfo).RunID))

		case agent.EventDecision:
			info := evt.Payload.(agent.DecisionInfo)
			fmt.Fprintf(os.Stderr, "\r  ⚠ Decision required\r\n")
			fmt.Fprintf(os.Stderr, "\r  %s\r\n", info.Question)
			fmt.Fprint(os.Stderr, styleInfo.Render("\r  ▸ "))

			answer, _ := bufio.NewReader(os.Stdin).ReadString('\n')
			answer = strings.TrimSpace(answer)
			if answer == "/quit" || answer == "/exit" || answer == "/q" {
				cancel()
				return currentWS
			}
			if answer == "" {
				answer = "继续"
			}
			if err := a.ResolveDecision(ctx, info.TicketID, "provide_missing_info", answer); err != nil {
				fmt.Fprintf(os.Stderr, "\r  error: %v\r\n", err)
				a.ClearNotified()
				continue
			}
			if err := a.ResumeRun(ctx, info.RunID); err != nil {
				fmt.Fprintf(os.Stderr, "\r  error: %v\r\n", err)
				a.ClearNotified()
				continue
			}
			fmt.Fprintf(os.Stderr, "\r  ...\r\n")

		case agent.EventApproval:
			info := evt.Payload.(agent.ApprovalInfo)
			fmt.Fprintf(os.Stderr, "\r  ⚡ Approval required\r\n")
			fmt.Fprintf(os.Stderr, "\r  %s\r\n", info.Reason)
			fmt.Fprint(os.Stderr, styleInfo.Render("\r  Approve? [y/N] "))

			answer, _ := bufio.NewReader(os.Stdin).ReadString('\n')
			answer = strings.TrimSpace(strings.ToLower(answer))
			if answer == "/quit" || answer == "/exit" || answer == "/q" {
				cancel()
				return currentWS
			}
			if err := a.ResolveApproval(ctx, info.ApprovalID, answer == "y"); err != nil {
				fmt.Fprintf(os.Stderr, "\r  error: %v\r\n", err)
				a.ClearNotified()
				continue
			}
			if err := a.ResumeRun(ctx, info.RunID); err != nil {
				fmt.Fprintf(os.Stderr, "\r  error: %v\r\n", err)
				a.ClearNotified()
				continue
			}
			fmt.Fprintf(os.Stderr, "\r  ...\r\n")

		case agent.EventRunCompleted:
			info, _ := evt.Payload.(agent.RunCompletedInfo)
			elapsed := time.Since(startedAt).Truncate(time.Second)
			fmt.Fprintf(os.Stderr, "\r  ✓ %s\r\n", styleDim.Render(elapsed.String()))
			fmt.Fprint(os.Stderr, "\r\n")

			for _, o := range info.Outputs {
				lines := strings.Split(o.Summary, "\n")
				for _, line := range lines {
					line = strings.TrimSpace(line)
					if line == "" {
						continue
					}
					if utf8.RuneCountInString(line) > 120 {
						line = string([]rune(line)[:117]) + "..."
					}
					fmt.Fprintf(os.Stderr, "\r  %s\r\n", line)
				}
			}
			if info.Summary != "" && len(info.Outputs) == 0 {
				fmt.Fprintf(os.Stderr, "\r  %s\r\n", info.Summary)
			}
			fmt.Fprint(os.Stderr, "\r\n")
			return currentWS

		case agent.EventRunFailed:
			elapsed := time.Since(startedAt).Truncate(time.Second)
			fmt.Fprintf(os.Stderr, "\r  ✗ %s\r\n", styleDim.Render(elapsed.String()))
			if evt.Error != nil {
				msg := evt.Error.Error()
				if strings.Contains(msg, "llm provider") || strings.Contains(msg, "empty") {
					fmt.Fprintf(os.Stderr, "\r  LLM 调用失败，请 /retry 重试\r\n")
				} else {
					fmt.Fprintf(os.Stderr, "\r  %s\r\n", msg)
				}
			}
			fmt.Fprint(os.Stderr, "\r\n")
			return currentWS

		case agent.EventCancelled:
			fmt.Fprintf(os.Stderr, "\r  interrupted\r\n\n")
			return currentWS
		}
	}
	return currentWS
}
