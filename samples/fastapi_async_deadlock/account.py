"""Concurrent banking transaction service suffering from an async race condition."""

import asyncio
import math
import threading
from typing import Dict


class BankLedger:
    def __init__(self, initial_balance: float = 1000.0):
        if initial_balance is None or isinstance(initial_balance, bool) or not isinstance(initial_balance, (int, float)):
            raise ValueError("Initial balance must be a numeric value")
        if math.isnan(initial_balance) or math.isinf(initial_balance) or initial_balance < 0:
            raise ValueError("Initial balance must be a non-negative finite number")
        self.balance = float(initial_balance)
        self._lock = threading.Lock()

    async def transfer(self, amount: float):
        """Simulate async transfer with non-atomic state mutation (Race Condition)."""
        if amount is None or isinstance(amount, bool) or not isinstance(amount, (int, float)):
            raise ValueError("Amount must be a numeric value")
        with self._lock:
            if math.isnan(amount) or math.isinf(amount) or amount <= 0:
                return
            if self.balance >= amount:
                self.balance -= amount

    async def deposit(self, amount: float):
        if amount is None or isinstance(amount, bool) or not isinstance(amount, (int, float)):
            raise ValueError("Amount must be a numeric value")
        with self._lock:
            if math.isnan(amount) or math.isinf(amount) or amount <= 0:
                return
            self.balance += amount
