# A simple refreshing work-cache

This is targeted at a use case where there are, perhaps, a few thousand
items that need to be cached. Each of those items takes some effort to
compute, so we share the work between callers to the cache.

Additionally, cache items may need to be refreshed (that is, computed anew);
that's the unique rub here. The cache keeps track of items and refreshes
them regularly. When it comes time to refresh an item, if it's not been used
then it's purged from the cache.

Conversely, as long as clients are asking for a value for a key, that value
will remain live, being refreshed periodically.

There's some additional support for negative caching. In practice, since
the computation that generates a result for a key is pluggable, and the
cache will just hang around quite happily waiting, it's the
refresher that should manage back-off internally (rather than having the
cache back off). In such a circumstance, the plugged refresher should probably
bump error counts to warn when it's getting in trouble; the cache is probably
best configured with a short negative cache period.

(It's just a simple experiment where cache entries are maintained by "live"
goroutines. I just wanted to see what it looked like.)