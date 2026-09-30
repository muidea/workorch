#!/usr/bin/env python3
"""对已启动服务验证持久创作链路；显式执行才会调用 provider，不自动启动 Run。"""
import argparse
import json
import time
import urllib.error
import urllib.request
import uuid


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base-url", default="http://127.0.0.1:8080")
    parser.add_argument("--template-id", required=True)
    parser.add_argument("--goal", required=True)
    parser.add_argument("--directory", default="")
    parser.add_argument("--resource", action="append", default=[])
    parser.add_argument("--requirement", action="append", default=[])
    parser.add_argument("--request-id", default=None, help="响应丢失后用同一身份与原输入重放")
    parser.add_argument("--timeout", type=int, default=300)
    parser.add_argument("--confirm", action="store_true", help="精确确认审查结果并要求 eligible；仍不启动 Run")
    args = parser.parse_args()
    if args.timeout < 1 or args.timeout > 3600:
        parser.error("timeout 必须在 1～3600 秒内")
    base = args.base_url.rstrip("/")
    if not base.startswith(("http://", "https://")):
        parser.error("base-url 必须为 HTTP(S)")

    def request(method, path, data=None):
        body = None if data is None else json.dumps(data, ensure_ascii=False).encode()
        req = urllib.request.Request(base + path, data=body, method=method, headers={"Content-Type": "application/json"})
        try:
            with urllib.request.urlopen(req, timeout=60) as response:
                return json.load(response)
        except urllib.error.HTTPError as error:
            # 不输出原响应、provider 文本或配置，避免泄漏敏感输入。
            raise RuntimeError(f"HTTP {error.code}: {method} {path}") from None

    request("GET", "/healthz")
    request("GET", "/readyz")
    request_id = args.request_id or str(uuid.uuid4())
    print(f"requestId={request_id}", flush=True)
    payload = {"requestId": request_id, "title": "规格创作 live 验证", "templateId": args.template_id,
               "deliverableDirectory": args.directory,
               "input": {"goalText": args.goal, "acceptanceRequirements": args.requirement, "resourceRefs": args.resource}}
    view = request("POST", "/api/v1/workorch/workspace-authoring", payload)["authoring"]
    workspace_id = view["workspaceId"]
    path = "/api/v1/workorch/workspaces/" + workspace_id
    print(f"workspaceId={workspace_id}", flush=True)
    replay = request("POST", "/api/v1/workorch/workspace-authoring", payload)["authoring"]
    if replay["workspaceId"] != workspace_id or replay["inputHash"] != view["inputHash"]:
        raise RuntimeError("重复创建没有返回相同持久身份")
    runs = request("GET", path + "/runs").get("runs", [])
    if runs:
        raise RuntimeError("创作阶段出现未显式启动的 Run")
    request("POST", path + "/authoring/compile")
    deadline = time.monotonic() + args.timeout
    while time.monotonic() < deadline:
        view = request("GET", path + "/authoring")["authoring"]
        if view["state"] in ("ready_to_confirm", "review_blocked", "failed", "clarification_required", "admitted"):
            break
        time.sleep(2)
    else:
        raise RuntimeError("编译超时或结果未决；不得以新请求自动重发 provider")
    print("state=" + view["state"], flush=True)
    if view["state"] != "ready_to_confirm":
        raise RuntimeError("未得到六审查通过的待确认规格；保留草稿供诊断")
    proposal = view["proposal"]
    reviews = proposal.get("reviews", [])
    if len(reviews) != 6 or len({review["kind"] for review in reviews}) != 6 or any(review["status"] != "PASS" for review in reviews):
        raise RuntimeError("六审查不完整或未全部 PASS")
    if not proposal.get("items"):
        raise RuntimeError("派生验收清单为空")
    if args.confirm:
        exact = {"inputHash": view["inputHash"], "derivationPermitId": view["derivationPermitId"],
                 "proposalHash": proposal["proposalHash"], "specificationHash": proposal["specificationHash"],
                 "expectedWorkspaceRevision": view["workspaceRevision"]}
        confirmed = request("POST", path + "/authoring/confirm", exact)["authoring"]
        if confirmed["state"] != "admitted" or not confirmed.get("startupInputHash"):
            raise RuntimeError("确认后未 eligible；请核对模板 proof 与准入阻塞")
        repeated = request("POST", path + "/authoring/confirm", exact)["authoring"]
        if repeated["startupInputHash"] != confirmed["startupInputHash"] or repeated["workspaceRevision"] != confirmed["workspaceRevision"]:
            raise RuntimeError("精确确认重放发生额外变更")
    if request("GET", path + "/runs").get("runs", []):
        raise RuntimeError("编译或确认自动创建 Run")
    print("PASS: 持久创建／六审查" + ("／确认准入重放" if args.confirm else "") + "／零自动 Run")


if __name__ == "__main__":
    try:
        main()
    except (RuntimeError, urllib.error.URLError, KeyError, ValueError) as error:
        raise SystemExit(f"FAIL: {error}") from None
