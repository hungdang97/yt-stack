import os
from datetime import datetime
from threading import Thread
from pymongo import MongoClient

# Error patterns indicating bad/expired cookies
BAD_COOKIE_ERRORS = ('sign in', 'login required', 'bot', 'please sign in', 'login', 'sign')

# MongoDB connection
_MONGO_URI = os.environ.get(
    'MONGO_URI',
    'mongodb://cookie:cookie123456789@85.10.196.119:27017/cookie'
)
_col = MongoClient(_MONGO_URI)['cookie']['cookies']


def _update_last_used(doc_id):
    """Background update - không block main thread."""
    _col.update_one({'_id': doc_id}, {'$set': {'last_used': datetime.now()}})


def get():
    """Get active cookie with least recent usage (round-robin)."""
    doc = _col.find_one({'status': 'active'}, sort=[('last_used', 1)])
    if doc:
        Thread(target=_update_last_used, args=(doc['_id'],), daemon=True).start()
        return doc['profile_name'], doc['cookie_string']
    return None, None


def get_batch(limit=10):
    """Get multiple active cookies for pool pre-fetching."""
    docs = list(_col.find({'status': 'active'}, sort=[('last_used', 1)]).limit(limit))
    if docs:
        # Update last_used for all fetched docs in background
        doc_ids = [doc['_id'] for doc in docs]
        Thread(target=_update_batch_last_used, args=(doc_ids,), daemon=True).start()
        return [(doc['profile_name'], doc['cookie_string']) for doc in docs]
    return []


def _update_batch_last_used(doc_ids):
    """Background batch update."""
    _col.update_many({'_id': {'$in': doc_ids}}, {'$set': {'last_used': datetime.now()}})


def invalidate(profile):
    """Mark a cookie profile as inactive."""
    _col.update_one(
        {'profile_name': profile},
        {'$set': {'status': 'inactive', 'updated_at': datetime.now()}}
    )


def is_bad(error):
    """Check if error indicates bad/expired cookie."""
    error_msg = str(error).lower()
    return any(pattern in error_msg for pattern in BAD_COOKIE_ERRORS)
