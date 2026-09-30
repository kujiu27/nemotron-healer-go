"""Distributed/Async lock primitive abstraction."""

import asyncio
from typing import Optional


class TransactionLock:
    """A non-reentrant async lock that deadlocks if a coroutine calls it re-entrantly."""

    def __init__(self):
        self._lock = asyncio.Lock()
        self._holder: Optional[int] = None

    async def acquire(self):
        # Non-reentrant lock: calling acquire twice in same task DEADLOCKS!
        await self._lock.acquire()

    def release(self):
        if self._lock.locked():
            self._lock.release()

    async def __aenter__(self):
        await self.acquire()
        return self

    async def __aexit__(self, exc_type, exc_val, exc_tb):
        self.release()
