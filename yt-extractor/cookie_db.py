import os
from datetime import datetime
from threading import Thread
from pymongo import MongoClient

# Error patterns indicating bad/expired cookies
BAD_COOKIE_ERRORS = ('sign in', 'login required', 'bot', 'please sign in', 'login', 'sign', 'page needs to be reloaded')

# MongoDB connection
_MONGO_URI = os.environ.get(
    'MONGO_URI',
    'mongodb://cookie:cookie123456789@85.10.196.119:27017/cookie'
)
_col = MongoClient(_MONGO_URI)['cookie']['cookies']


def get():
    """Get a random active cookie."""
    pipeline = [
        {'$match': {'status': 'active'}},
        {'$sample': {'size': 1}}
    ]
    docs = list(_col.aggregate(pipeline))
    if docs:
        doc = docs[0]
        # No last_used update needed for random strategy
        return doc['profile_name'], doc['cookie_string']
    return None, None


def get_batch(limit=10):
    """Get multiple random active cookies."""
    pipeline = [
        {'$match': {'status': 'active'}},
        {'$sample': {'size': limit}}
    ]
    docs = list(_col.aggregate(pipeline))
    if docs:
        return [(doc['profile_name'], doc['cookie_string']) for doc in docs]
    return []


def _update_batch_last_used(doc_ids):
    """Deprecated: No longer used with random strategy."""
    pass


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
