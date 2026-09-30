"""Audit logging module that tracks all transactions."""

import time
from typing import List, Dict, Any


class TransactionAuditor:
    def __init__(self):
        self.audit_log: List[Dict[str, Any]] = []

    def record(self, account_id: str, delta: float, balance: float):
        # Synchronous CPU/IO simulation
        time.sleep(0.001)
        self.audit_log.append({
            "account_id": account_id,
            "delta": delta,
            "balance": balance,
            "timestamp": time.time(),
        })

    def count(self) -> int:
        return len(self.audit_log)
