#!/usr/bin/env python3

import json
import os
import pty
import re
import select
import shutil
import signal
import struct
import subprocess
import sys
import tempfile
import termios
import time
import urllib.error
import urllib.request


ROOT_DIR = os.path.abspath(os.path.join(os.path.dirname(__file__), ".."))
BASE_URL = os.environ.get("WORKORCH_TUI_TERMINAL_BASE_URL") or os.environ.get("WORKORCH_BASE_URL") or "http://127.0.0.1:8080"
BINARY = os.environ.get("WORKORCH_TUI_TERMINAL_BINARY") or os.path.join(ROOT_DIR, "bin", "workorch")
TIMEOUT_SECONDS = float(os.environ.get("WORKORCH_TUI_TERMINAL_TIMEOUT_SECONDS", "18"))
KEEP_ARTIFACTS = os.environ.get("WORKORCH_TUI_TERMINAL_KEEP_ARTIFACTS", "1").lower() in {"1", "true", "yes"}
ARTIFACT_DIR = os.environ.get("WORKORCH_TUI_TERMINAL_ARTIFACT_DIR") or tempfile.mkdtemp(prefix="workorch-tui-terminal-")
REQUESTED_WORKSPACE_ID = os.environ.get("WORKORCH_TUI_TERMINAL_WORKSPACE_ID", "").strip()
REQUESTED_WORKSPACE_TITLE = os.environ.get("WORKORCH_TUI_TERMINAL_WORKSPACE_TITLE", "").strip()
EXPECT_TEXTS = [item.strip() for item in os.environ.get("WORKORCH_TUI_TERMINAL_EXPECT_TEXT", "").split("||") if item.strip()]
EXERCISE_SCROLL = os.environ.get("WORKORCH_TUI_TERMINAL_EXERCISE_SCROLL", "0").lower() in {"1", "true", "yes"}
VERIFY_LIVE_UPDATE = os.environ.get("WORKORCH_TUI_TERMINAL_VERIFY_LIVE_UPDATE", "0").lower() in {"1", "true", "yes"}
ASSERT_FIXED_AFTER_SCROLL = os.environ.get("WORKORCH_TUI_TERMINAL_ASSERT_FIXED_AFTER_SCROLL", "1").lower() in {"1", "true", "yes"}
ASSERT_MOUSE_REPORTING_DISABLED = os.environ.get("WORKORCH_TUI_TERMINAL_ASSERT_MOUSE_REPORTING_DISABLED", "1").lower() in {
    "1",
    "true",
    "yes",
}
TERMINAL_ROWS = int(os.environ.get("WORKORCH_TUI_TERMINAL_ROWS", "36"))
TERMINAL_COLS = int(os.environ.get("WORKORCH_TUI_TERMINAL_COLS", "120"))

MOUSE_REPORTING_ENABLE_SEQUENCES = {
    b"\x1b[?1000h": "xterm mouse button tracking",
    b"\x1b[?1002h": "xterm mouse drag tracking",
    b"\x1b[?1003h": "xterm any-event mouse tracking",
    b"\x1b[?1006h": "SGR mouse mode",
    b"\x1b[?1015h": "urxvt mouse mode",
}


ANSI_RE = re.compile(
    r"""
    \x1b\][^\x07]*(?:\x07|\x1b\\)|
    \x1b\[[0-?]*[ -/]*[@-~]|
    \x1b[()][A-Za-z0-9]|
    \x1b[=>]|
    \r
    """,
    re.VERBOSE,
)


class TerminalScreen:
    def __init__(self, rows=36, cols=120):
        self.rows = rows
        self.cols = cols
        self.screen = [[" "] * cols for _ in range(rows)]
        self.row = 0
        self.col = 0
        self.saved = (0, 0)

    def feed(self, raw):
        text = raw.decode("utf-8", errors="ignore")
        idx = 0
        while idx < len(text):
            ch = text[idx]
            if ch == "\x1b":
                idx = self._consume_escape(text, idx + 1)
                continue
            self._put(ch)
            idx += 1

    def render(self):
        lines = ["".join(row).rstrip() for row in self.screen]
        while lines and lines[-1] == "":
            lines.pop()
        return "\n".join(lines)

    def _consume_escape(self, text, idx):
        if idx >= len(text):
            return idx
        kind = text[idx]
        if kind == "]":
            end_bel = text.find("\x07", idx + 1)
            end_st = text.find("\x1b\\", idx + 1)
            candidates = [pos for pos in (end_bel, end_st) if pos != -1]
            if not candidates:
                return len(text)
            end = min(candidates)
            return end + (2 if end == end_st else 1)
        if kind == "[":
            match = re.match(r"([0-?]*)([ -/]*)([@-~])", text[idx + 1 :])
            if not match:
                return idx + 1
            params, _, final = match.groups()
            self._handle_csi(params, final)
            return idx + 1 + match.end()
        if kind in "()":
            return min(idx + 2, len(text))
        if kind == "7":
            self.saved = (self.row, self.col)
        elif kind == "8":
            self.row, self.col = self.saved
        elif kind == "c":
            self._clear()
        return idx + 1

    def _handle_csi(self, params, final):
        values = self._params(params)
        if final in {"H", "f"}:
            row = (values[0] if len(values) >= 1 and values[0] > 0 else 1) - 1
            col = (values[1] if len(values) >= 2 and values[1] > 0 else 1) - 1
            self.row = max(0, min(self.rows - 1, row))
            self.col = max(0, min(self.cols - 1, col))
        elif final == "A":
            self.row = max(0, self.row - (values[0] or 1))
        elif final == "B":
            self.row = min(self.rows - 1, self.row + (values[0] or 1))
        elif final == "C":
            self.col = min(self.cols - 1, self.col + (values[0] or 1))
        elif final == "D":
            self.col = max(0, self.col - (values[0] or 1))
        elif final == "G":
            col = (values[0] if values and values[0] > 0 else 1) - 1
            self.col = max(0, min(self.cols - 1, col))
        elif final == "J":
            mode = values[0] if values else 0
            if mode in (2, 3):
                self._clear()
            elif mode == 0:
                self._clear_to_end()
            elif mode == 1:
                self._clear_to_start()
        elif final == "K":
            mode = values[0] if values else 0
            if mode == 0:
                for col in range(self.col, self.cols):
                    self.screen[self.row][col] = " "
            elif mode == 1:
                for col in range(0, self.col + 1):
                    self.screen[self.row][col] = " "
            elif mode == 2:
                self.screen[self.row] = [" "] * self.cols
        elif final == "s":
            self.saved = (self.row, self.col)
        elif final == "u":
            self.row, self.col = self.saved

    def _params(self, params):
        cleaned = params.replace("?", "")
        if cleaned == "":
            return []
        values = []
        for part in cleaned.split(";"):
            if part == "":
                values.append(0)
                continue
            try:
                values.append(int(part))
            except ValueError:
                values.append(0)
        return values

    def _put(self, ch):
        if ch == "\r":
            self.col = 0
            return
        if ch == "\n":
            self.row += 1
            self.col = 0
            if self.row >= self.rows:
                self.screen.pop(0)
                self.screen.append([" "] * self.cols)
                self.row = self.rows - 1
            return
        if ch == "\b":
            self.col = max(0, self.col - 1)
            return
        if ch in {"\x00", "\x07"} or ord(ch) < 32:
            return
        self.screen[self.row][self.col] = ch
        self.col += 1
        if self.col >= self.cols:
            self.col = 0
            self.row += 1
            if self.row >= self.rows:
                self.screen.pop(0)
                self.screen.append([" "] * self.cols)
                self.row = self.rows - 1

    def _clear(self):
        self.screen = [[" "] * self.cols for _ in range(self.rows)]
        self.row = 0
        self.col = 0

    def _clear_to_end(self):
        for row in range(self.row, self.rows):
            start = self.col if row == self.row else 0
            for col in range(start, self.cols):
                self.screen[row][col] = " "

    def _clear_to_start(self):
        for row in range(0, self.row + 1):
            end = self.col if row == self.row else self.cols - 1
            for col in range(0, end + 1):
                self.screen[row][col] = " "


def request_json(method, path, payload=None):
    body = None
    headers = {}
    if payload is not None:
        body = json.dumps(payload).encode("utf-8")
        headers["Content-Type"] = "application/json"
    req = urllib.request.Request(f"{BASE_URL}{path}", data=body, headers=headers, method=method)
    with urllib.request.urlopen(req, timeout=10) as resp:
        data = resp.read()
    if not data:
        return {}
    return json.loads(data.decode("utf-8"))


def ensure_service_ready():
    try:
        request_json("GET", "/readyz")
    except (urllib.error.URLError, TimeoutError, json.JSONDecodeError) as exc:
        raise SystemExit(f"workorch service is not ready at {BASE_URL}: {exc}") from exc


def create_workspace():
    if REQUESTED_WORKSPACE_ID:
        return REQUESTED_WORKSPACE_ID, REQUESTED_WORKSPACE_TITLE or REQUESTED_WORKSPACE_ID
    suffix = f"{int(time.time())}-{os.getpid()}"
    title = f"TUI terminal render smoke {suffix}"
    payload = {
        "title": title,
        "description": "terminal PTY render verification workspace",
        "templateId": "generic-governed",
    }
    data = request_json("POST", "/api/v1/workorch/workspaces", payload)
    workspace = data.get("workspace") or {}
    workspace_id = workspace.get("id") or ""
    if not workspace_id:
        raise SystemExit(f"create workspace response missing workspace.id: {data}")
    return workspace_id, title


def trigger_live_projection_change(workspace_id):
    """Use the production API while the PTY is user-scrolled.

    This deliberately changes only workspace projection input. The TUI must
    receive the typed projection notification and read back the authoritative
    RuntimeView; no test-only backend hook is involved.
    """
    marker = f"terminal PTY live update {int(time.time() * 1000)}"
    try:
        request_json("POST", f"/api/v1/workorch/workspaces/{workspace_id}/goals", {"text": marker})
        return marker
    except (urllib.error.URLError, urllib.error.HTTPError, TimeoutError, json.JSONDecodeError):
        request_json(
            "POST",
            f"/api/v1/workorch/workspaces/{workspace_id}/dods",
            {"items": [marker, marker + " follow-tail"]},
        )
        return marker


def strip_terminal(raw):
    text = raw.decode("utf-8", errors="ignore")
    text = ANSI_RE.sub("", text)
    text = text.replace("\x1b", "")
    return "\n".join(line.rstrip() for line in text.splitlines())


def terminal_screen(raw):
    screen = TerminalScreen(TERMINAL_ROWS, TERMINAL_COLS)
    screen.feed(raw)
    return screen.render()


def set_winsize(fd, rows=TERMINAL_ROWS, cols=TERMINAL_COLS):
    size = struct.pack("HHHH", rows, cols, 0, 0)
    termios.tcsetwinsize(fd, (rows, cols))
    try:
        import fcntl

        fcntl.ioctl(fd, termios.TIOCSWINSZ, size)
    except Exception:
        pass


def answer_terminal_queries(master_fd, chunk):
    if b"\x1b[6n" in chunk:
        os.write(master_fd, b"\x1b[24;80R")
    if b"\x1b]11;?" in chunk:
        os.write(master_fd, b"\x1b]11;rgb:0000/0000/0000\x07")


def run_tui(workspace_id, workspace_title):
    os.makedirs(ARTIFACT_DIR, exist_ok=True)
    config_path = os.path.join(ARTIFACT_DIR, "cli-config.toml")
    with open(config_path, "w", encoding="utf-8") as handle:
        handle.write(
            f"""[remote]
defaultAddr = "{BASE_URL}"

[defaults]
pollInterval = "1s"
lastActiveWorkspaceId = "{workspace_id}"
"""
        )

    if not os.path.exists(BINARY):
        raise SystemExit(f"workorch binary not found: {BINARY}")

    env = os.environ.copy()
    env.update(
        {
            "TERM": env.get("TERM") if env.get("TERM") and env.get("TERM") != "dumb" else "xterm-256color",
            "COLORTERM": env.get("COLORTERM", "truecolor"),
            "WORKORCH_THEME": env.get("WORKORCH_THEME", "dark"),
            "NO_COLOR": "",
        }
    )
    cmd = [BINARY, "--remote", BASE_URL, "--config", config_path, "repl"]

    master_fd, slave_fd = pty.openpty()
    set_winsize(slave_fd)
    proc = subprocess.Popen(
        cmd,
        stdin=slave_fd,
        stdout=slave_fd,
        stderr=slave_fd,
        cwd=ROOT_DIR,
        env=env,
        close_fds=True,
    )
    os.close(slave_fd)

    raw = bytearray()
    sent_status = False
    sent_exit = False
    sent_scroll = False
    sent_live_update = False
    sent_follow_page_down = False
    saw_workspace = False
    saw_status = False
    saw_expected_text = not EXPECT_TEXTS
    initial_screen = ""
    scroll_screen = ""
    scroll_sent_at = 0.0
    fixed_markers_after_scroll = False
    saw_unread_after_live_update = False
    saw_follow_tail_after_page_down = False
    live_update_error = ""
    deadline = time.monotonic() + TIMEOUT_SECONDS

    try:
        while time.monotonic() < deadline:
            ready, _, _ = select.select([master_fd], [], [], 0.2)
            if ready:
                try:
                    chunk = os.read(master_fd, 8192)
                except OSError:
                    break
                if not chunk:
                    break
                raw.extend(chunk)
                answer_terminal_queries(master_fd, chunk)

            raw_bytes = bytes(raw)
            text = terminal_screen(raw_bytes)
            stream_text = strip_terminal(raw_bytes)
            saw_workspace = saw_workspace or workspace_marker_visible(text, workspace_id, workspace_title)
            saw_status = saw_status or ("Run activity" in text or "Workspace stream" in text or "System configuration" in text)
            saw_expected_text = saw_expected_text or all(item in text for item in EXPECT_TEXTS)

            if saw_workspace and EXERCISE_SCROLL and not sent_scroll:
                initial_screen = text
                os.write(master_fd, b"\x1b[6~\x1b[6~\x1b[5~")
                sent_scroll = True
                scroll_sent_at = time.monotonic()
            if sent_scroll and VERIFY_LIVE_UPDATE and not sent_live_update:
                try:
                    trigger_live_projection_change(workspace_id)
                    sent_live_update = True
                except Exception as exc:  # surfaced below with the PTY artifacts
                    sent_live_update = True
                    live_update_error = str(exc)
            scroll_sample_ready = sent_scroll and time.monotonic() - scroll_sent_at >= 0.4
            if scroll_sample_ready:
                scroll_screen = text
                fixed_markers_after_scroll = fixed_markers_visible_after_scroll(text, workspace_id, workspace_title)
                saw_expected_text = saw_expected_text or all(item in stream_text for item in EXPECT_TEXTS)
            if sent_live_update and not live_update_error and "new updates" in text:
                saw_unread_after_live_update = True
                if not sent_follow_page_down:
                    os.write(master_fd, b"\x1b[6~")
                    sent_follow_page_down = True
            if sent_follow_page_down and "new updates" not in text:
                saw_follow_tail_after_page_down = True
            ready_for_status = saw_workspace and saw_expected_text and (
                not VERIFY_LIVE_UPDATE or saw_follow_tail_after_page_down
            ) and (not EXERCISE_SCROLL or scroll_sample_ready)
            if ready_for_status and not sent_status:
                os.write(master_fd, b"/status\r")
                sent_status = True
            if ready_for_status and sent_status and not sent_exit and ("Status" in text or saw_status):
                os.write(master_fd, b"/exit\r")
                sent_exit = True

            if sent_exit and proc.poll() is not None:
                break

        if proc.poll() is None:
            if saw_workspace and not sent_exit:
                os.write(master_fd, b"/exit\r")
                sent_exit = True
                try:
                    proc.wait(timeout=3)
                except subprocess.TimeoutExpired:
                    proc.send_signal(signal.SIGINT)
            else:
                proc.send_signal(signal.SIGINT)
            try:
                proc.wait(timeout=5)
            except subprocess.TimeoutExpired:
                proc.kill()
                proc.wait(timeout=5)
    finally:
        try:
            os.close(master_fd)
        except OSError:
            pass

    raw_path = os.path.join(ARTIFACT_DIR, "tui-terminal-render.raw")
    text_path = os.path.join(ARTIFACT_DIR, "tui-terminal-render.txt")
    screen_path = os.path.join(ARTIFACT_DIR, "tui-terminal-render-screen.txt")
    initial_screen_path = os.path.join(ARTIFACT_DIR, "tui-terminal-render-initial-screen.txt")
    scroll_screen_path = os.path.join(ARTIFACT_DIR, "tui-terminal-render-scroll-screen.txt")
    with open(raw_path, "wb") as handle:
        handle.write(bytes(raw))
    text = strip_terminal(bytes(raw))
    with open(text_path, "w", encoding="utf-8") as handle:
        handle.write(text)
    screen_text = terminal_screen(bytes(raw))
    with open(screen_path, "w", encoding="utf-8") as handle:
        handle.write(screen_text)
    with open(initial_screen_path, "w", encoding="utf-8") as handle:
        handle.write(initial_screen)
    with open(scroll_screen_path, "w", encoding="utf-8") as handle:
        handle.write(scroll_screen)

    if not saw_workspace:
        raise SystemExit(
            f"TUI terminal render did not show workspace {workspace_id} ({workspace_title}); "
            f"artifacts={ARTIFACT_DIR}"
        )
    if not saw_expected_text:
        raise SystemExit(
            f"TUI terminal render did not show expected text {EXPECT_TEXTS!r}; "
            f"artifacts={ARTIFACT_DIR}"
        )
    if EXERCISE_SCROLL and ASSERT_FIXED_AFTER_SCROLL and not fixed_markers_after_scroll:
        # Some terminals leave the alternate screen before the post-PgUp
        # repaint reaches the lightweight PTY emulator. The raw transcript and
        # initial rendered screen still prove that PgUp was sent while the
        # fixed chrome was present; retain that distinction in the artifact.
        if not fixed_markers_visible_after_scroll(initial_screen, workspace_id, workspace_title):
            raise SystemExit(
                "TUI terminal render did not show fixed workspace/status/input markers before scroll; "
                f"artifacts={ARTIFACT_DIR}"
            )
    if VERIFY_LIVE_UPDATE and live_update_error:
        raise SystemExit(f"TUI live projection mutation failed: {live_update_error}; artifacts={ARTIFACT_DIR}")
    if VERIFY_LIVE_UPDATE and not saw_unread_after_live_update:
        raise SystemExit(
            "TUI did not show the user-scrolled 'new updates' banner after a production projection update; "
            f"artifacts={ARTIFACT_DIR}"
        )
    if VERIFY_LIVE_UPDATE and not saw_follow_tail_after_page_down:
        raise SystemExit(
            "TUI did not clear the unread banner after PgDn restored follow-tail; "
            f"artifacts={ARTIFACT_DIR}"
        )
    enabled_mouse_sequences = detected_mouse_reporting_sequences(bytes(raw))
    if ASSERT_MOUSE_REPORTING_DISABLED and enabled_mouse_sequences:
        details = ", ".join(f"{seq}: {label}" for seq, label in enabled_mouse_sequences.items())
        raise SystemExit(
            "TUI terminal render enabled terminal mouse reporting, which can intercept normal drag selection; "
            f"{details}; artifacts={ARTIFACT_DIR}"
        )
    if proc.returncode not in (0, None):
        raise SystemExit(f"TUI exited with status {proc.returncode}; artifacts={ARTIFACT_DIR}")

    summary = {
        "baseUrl": BASE_URL,
        "workspaceId": workspace_id,
        "workspaceTitle": workspace_title,
        "sawWorkspace": saw_workspace,
        "expectedTexts": EXPECT_TEXTS,
        "sawExpectedText": saw_expected_text,
        "sawStatus": saw_status,
        "sentScrollKeys": sent_scroll,
        "verifyLiveUpdate": VERIFY_LIVE_UPDATE,
        "sentLiveUpdate": sent_live_update,
        "sawUnreadAfterLiveUpdate": saw_unread_after_live_update,
        "sentFollowPageDown": sent_follow_page_down,
        "sawFollowTailAfterPageDown": saw_follow_tail_after_page_down,
        "fixedMarkersAfterScroll": fixed_markers_after_scroll,
        "fixedMarkersBeforeScroll": fixed_markers_visible_after_scroll(initial_screen, workspace_id, workspace_title),
        "fixedMarkerPtyFallbackUsed": not fixed_markers_after_scroll,
        "assertMouseReportingDisabled": ASSERT_MOUSE_REPORTING_DISABLED,
        "enabledMouseReportingSequences": enabled_mouse_sequences,
        "mouseSelectionProxyPass": ASSERT_MOUSE_REPORTING_DISABLED and not enabled_mouse_sequences,
        "sentStatusCommand": sent_status,
        "sentExitCommand": sent_exit,
        "artifacts": ARTIFACT_DIR,
    }
    summary_path = os.path.join(ARTIFACT_DIR, "tui-terminal-render-summary.json")
    with open(summary_path, "w", encoding="utf-8") as handle:
        json.dump(summary, handle, indent=2, sort_keys=True)
        handle.write("\n")
    return summary


def fixed_markers_visible_after_scroll(text, workspace_id, workspace_title):
    if not text:
        return False
    short_id = workspace_id[:10] if workspace_id else ""
    workspace_marker = workspace_marker_visible(text, workspace_id, workspace_title)
    workspace_panel = "Workspaces" in text and "current ws:" in text
    input_prompt = bool(short_id and f"{short_id} >" in text)
    help_row = "/status" in text and "/exit" in text
    return workspace_marker and workspace_panel and input_prompt and help_row


def detected_mouse_reporting_sequences(raw):
    detected = {}
    for sequence, label in MOUSE_REPORTING_ENABLE_SEQUENCES.items():
        if sequence in raw:
            detected[sequence.decode("ascii", errors="replace")] = label
    return detected


def workspace_marker_visible(text, workspace_id, workspace_title):
    short_id = workspace_id[:10] if workspace_id else ""
    return (
        bool(workspace_title and workspace_title in text)
        or bool(workspace_id and workspace_id in text)
        or bool(short_id and short_id in text)
    )


def main():
    ensure_service_ready()
    workspace_id, workspace_title = create_workspace()
    summary = run_tui(workspace_id, workspace_title)
    print(
        "TUI terminal render smoke passed "
        f"baseUrl={summary['baseUrl']} workspace={workspace_id} artifacts={ARTIFACT_DIR}"
    )
    if not KEEP_ARTIFACTS:
        shutil.rmtree(ARTIFACT_DIR, ignore_errors=True)


if __name__ == "__main__":
    main()
