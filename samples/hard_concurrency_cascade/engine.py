"""Original broken engine.py (re-entrancy unsafe)."""

import asyncio
from locks import TransactionLock
from auditor import TransactionAuditor


class LedgerEngine:
    def __init__(self, initial_balance: float = 1000.0):
        self.balance = initial_balance
        self.lock = TransactionLock()
        self.auditor = TransactionAuditor()

    async def mutate_balance(self, account_id: str, delta: float):
        """Mutate balance without proper re-entrant synchronization (RACE CONDITION!)."""
        curr = self.balance
        # Simulate asynchronous I/O context switch
        await asyncio.sleep(0.002)
        self.balance = curr + delta
        self.auditor.record(account_id, delta, self.balance)
        return self.balance
