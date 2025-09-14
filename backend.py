#!/usr/bin/env python3
# -*- coding: utf-8 -*-

from flask import Flask, request, jsonify, render_template
from flask_cors import CORS
import subprocess
import json
import os
import base64
import logging
from datetime import datetime

# =========================
# 基本配置（按需修改）
# =========================
CONTRACT_NAME = os.getenv("CM_CONTRACT", "CMNFA")
SDK_CONF_PATH = os.getenv("CM_SDK", "./testdata/sdk_config.yml")
CMC_BIN = os.getenv("CM_CMC_BIN", "./cmc")  # 相对工作目录
WORK_DIR = os.getenv("CM_WORKDIR", "/home/young3/chainmaker-go/tools/cmc")

# Flask
app = Flask(__name__, template_folder="templates", static_folder="static")
CORS(app)
logging.basicConfig(level=logging.INFO, format="%(asctime)s [%(levelname)s] %(message)s")
log = logging.getLogger("cmnfa")

# =========================
# 工具函数
# =========================
def _is_base64(s: str) -> bool:
    try:
        # 粗略判断：长度与字符集
        if not s or any(c not in "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/=\n\r" for c in s):
            return False
        base64.b64decode(s, validate=True)
        return True
    except Exception:
        return False

def _decode_result(maybe_b64: str) -> str:
    if isinstance(maybe_b64, str) and _is_base64(maybe_b64):
        try:
            return base64.b64decode(maybe_b64).decode("utf-8")
        except Exception:
            return maybe_b64
    return maybe_b64

def exec_cmc(method: str, params: dict | None = None, sync: bool = True, timeout=60):
    """
    调用：cmc client contract user invoke
    说明：本机使用 sdk_config.yml 指定的默认用户签名发起交易/查询。
    """
    cmd = [
        CMC_BIN, "client", "contract", "user", "invoke",
        f"--contract-name={CONTRACT_NAME}",
        f"--method={method}",
        f"--sdk-conf-path={SDK_CONF_PATH}",
        f"--sync-result={'true' if sync else 'false'}",
    ]
    if params:
        # 用紧凑 JSON，避免 shell 解析问题
        cmd.append("--params=" + json.dumps(params, separators=(',', ':')))

    if not os.path.exists(os.path.join(WORK_DIR, os.path.basename(CMC_BIN))):
        return False, f"找不到 CMC 可执行文件：{os.path.join(WORK_DIR, CMC_BIN)}"
    if not os.path.exists(os.path.join(WORK_DIR, SDK_CONF_PATH)):
        return False, f"找不到 SDK 配置：{os.path.join(WORK_DIR, SDK_CONF_PATH)}"

    log.info("Exec: %s", " ".join(cmd))
    try:
        p = subprocess.run(cmd, cwd=WORK_DIR, capture_output=True, text=True, timeout=timeout)
        stdout = (p.stdout or "").strip()
        stderr = (p.stderr or "").strip()
        if p.returncode != 0:
            return False, stderr or stdout or f"cmc 退出码 {p.returncode}"
        # stdout 一般就是 JSON
        try:
            data = json.loads(stdout)
        except Exception:
            data = stdout
        return True, data
    except subprocess.TimeoutExpired:
        return False, "执行超时"
    except Exception as e:
        return False, f"执行异常：{e}"

def ok(data): return jsonify({"success": True, "data": data})
def err(msg): return jsonify({"success": False, "error": msg})

# =========================
# 页面
# =========================
@app.get("/")
def home():
    return render_template("index.html", contract=CONTRACT_NAME)

# =========================
# 系统与基础查询
# =========================
@app.get("/api/system/status")
def system_status():
    cmc_ok = os.path.exists(os.path.join(WORK_DIR, os.path.basename(CMC_BIN)))
    cfg_ok = os.path.exists(os.path.join(WORK_DIR, SDK_CONF_PATH))
    return ok({
        "cmc": "OK" if cmc_ok else "MISSING",
        "sdk_config": "OK" if cfg_ok else "MISSING",
        "contract": CONTRACT_NAME,
        "workdir": WORK_DIR,
        "time": datetime.now().isoformat()
    })

@app.get("/api/nfa/total-supply")
def total_supply():
    ok_, data = exec_cmc("TotalSupply", params=None, sync=True)
    if not ok_:
        return err(data)
    # data -> {"contract_result":{"result": "MA==", "message":"Success",...}}
    res = data.get("contract_result", {}).get("result", "MA==")
    return ok(_decode_result(res))

@app.post("/api/nfa/owner")
def owner_of():
    body = request.get_json(force=True)
    token_id = body.get("tokenId", "").strip()
    if not token_id:
        return err("tokenId 不能为空")
    ok_, data = exec_cmc("OwnerOf", {"tokenId": token_id})
    if not ok_:
        return err(data)
    res = data.get("contract_result", {}).get("result", "")
    return ok(_decode_result(res))

@app.post("/api/nfa/token-uri")
def token_uri():
    body = request.get_json(force=True)
    token_id = body.get("tokenId", "").strip()
    if not token_id:
        return err("tokenId 不能为空")
    ok_, data = exec_cmc("TokenURI", {"tokenId": token_id})
    if not ok_:
        return err(data)
    res = data.get("contract_result", {}).get("result", "")
    return ok(_decode_result(res))

@app.post("/api/nfa/balance-of")
def balance_of():
    body = request.get_json(force=True)
    account = body.get("account", "").strip()
    if not account:
        return err("account 不能为空")
    ok_, data = exec_cmc("BalanceOf", {"account": account})
    if not ok_:
        return err(data)
    res = data.get("contract_result", {}).get("result", "MA==")
    return ok(_decode_result(res))

# =========================
# 业务操作：Mint / TransferFrom / Burn / 类别
# =========================
@app.post("/api/nfa/mint")
def mint():
    """
    仅合约内置管理员地址（state 中 admin）可操作。
    注意：调用者由 sdk_config.yml 指定的用户决定。
    """
    b = request.get_json(force=True)
    to = b.get("to", "").strip()
    token_id = b.get("tokenId", "").strip()
    category = b.get("categoryName", "").strip()
    metadata_text = (b.get("metadata_text") or "").encode("utf-8")
    metadata_b64 = b.get("metadata_b64")

    if not (to and token_id and category):
        return err("to / tokenId / categoryName 不能为空")

    if metadata_b64:
        meta = metadata_b64
    else:
        # 允许传明文，后端代为 base64
        meta = base64.b64encode(metadata_text).decode("utf-8") if metadata_text else ""

    ok_, data = exec_cmc("Mint", {"to": to, "tokenId": token_id, "categoryName": category, "metadata": meta})
    if not ok_:
        return err(data)

    # 取事件与提示
    cr = data.get("contract_result", {})
    msg = cr.get("message", "")
    evts = cr.get("contract_event", [])
    pretty = {
        "message": msg,
        "events": evts,
        "tx_id": data.get("tx_id"),
        "block_height": data.get("tx_block_height")
    }
    return ok(pretty)

@app.post("/api/nfa/transfer-from")
def transfer_from():
    b = request.get_json(force=True)
    from_addr = b.get("from", "").strip()
    to_addr = b.get("to", "").strip()
    token_id = b.get("tokenId", "").strip()
    if not (from_addr and to_addr and token_id):
        return err("from / to / tokenId 不能为空")

    ok_, data = exec_cmc("TransferFrom", {"from": from_addr, "to": to_addr, "tokenId": token_id})
    if not ok_:
        return err(data)
    cr = data.get("contract_result", {})
    pretty = {
        "message": cr.get("message", ""),
        "events": cr.get("contract_event", []),
        "tx_id": data.get("tx_id"),
        "block_height": data.get("tx_block_height")
    }
    return ok(pretty)

@app.post("/api/nfa/burn")
def burn():
    b = request.get_json(force=True)
    token_id = b.get("tokenId", "").strip()
    if not token_id:
        return err("tokenId 不能为空")
    ok_, data = exec_cmc("Burn", {"tokenId": token_id})
    if not ok_:
        return err(data)
    cr = data.get("contract_result", {})
    pretty = {
        "message": cr.get("message", ""),
        "events": cr.get("contract_event", []),
        "tx_id": data.get("tx_id"),
        "block_height": data.get("tx_block_height")
    }
    return ok(pretty)

@app.post("/api/nfa/create-or-set-category")
def create_or_set_category():
    """
    给分类设置 URI（或新建分类）。
    请求体示例：
    {
      "categoryName": "demo",
      "categoryURI": "https://example.org/nfa"
    }
    """
    b = request.get_json(force=True)
    name = b.get("categoryName", "").strip()
    uri = b.get("categoryURI", "").strip()
    if not (name and uri):
        return err("categoryName / categoryURI 不能为空")

    category_json = json.dumps({"categoryName": name, "categoryURI": uri})
    ok_, data = exec_cmc("CreateOrSetCategory", {"category": category_json})
    if not ok_:
        return err(data)
    cr = data.get("contract_result", {})
    return ok({
        "message": cr.get("message", ""),
        "events": cr.get("contract_event", []),
        "tx_id": data.get("tx_id"),
        "block_height": data.get("tx_block_height")
    })

# =========================
# 入口
# =========================
if __name__ == "__main__":
    print(f"🚀 CMNFA backend | contract={CONTRACT_NAME}")
    print(f"📁 workdir: {WORK_DIR}")
    print(f"🔧 sdk:     {SDK_CONF_PATH}")
    app.run(host="0.0.0.0", port=5000, debug=True)
