"""Core ledger engine mutating balances and calling auditor."""

import asyncio
from locks import TransactionLock
from auditor import TransactionAuditor


class ReentrantTransactionLock(TransactionLock):
    def __init__(self):
        super().__init__()
        self._holder = None
        self._count = 0

    async def acquire(self):
        current_task = asyncio.current_task()
        if self._holder == current_task:
            self._count += 1
            return
        await super().acquire()
        self._holder = current_task
        self._count = 1

    def release(self):
        current_task = asyncio.current_task()
        if self._holder != current_task:
            return
        self._count -= 1
        if self._count == 0:
            self._holder = None
            super().release()


class LedgerEngine:
    def __init__(self, initial_balance: float = 1000.0):
        self.balance = initial_balance
        self.lock = ReentrantTransactionLock()
        self.auditor = TransactionAuditor()

    async def mutate_balance(self, account_id: str, delta: float):
        """Mutate balance with proper re-entrant synchronization."""
        async with self.lock:
            curr = self.balance
            # Simulate asynchronous I/O context switch
            await asyncio.sleep(0.002)
            self.balance = curr + delta
            self.auditor.record(account_id, delta, self.balance)
            return self.balance
