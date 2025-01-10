gemipfs is an experimental content-addressed transport & privacy layer for {gemini, http}

Query:
HTTP Request gets hashed to a "RequestCID".
Request CID has a derived-hash of a QueryCID.

Caching check:
The client asks about a QueryCID against known attestions mapping that QueryCID to known response objects.

Response object looks like:
HTTP Response (encrypted to RequestCID)
+ Attestation("QueryCID is answered by ResponseCID")


Setup:
User trusts / pins a service for 'exit' capabilities.

Subsystem: Discovery
1. Pubsub to find nodes offering exit service that have a ucan valid from the service.
2. Query IPNI on domain-canonical QueryCID's to find data repos, then query them for full canonicalized QueryCID
    Note: perhaps as with resoluton subsystem, there's an attestation-extensibility point here about the right 'roots' to use to identify appropriate data repos to query
3. If data not cached, follow 'Browsing' to get new data.


Subsystem: Resolution
1. User idenitification & resolution of canonicalization objects:
    Attestion(QueryCID canonical to Query'CID)
2. Some modules for this maybe done locally (e.g. adblock, user-agent-header stripping)
3. Some could be done by a publisher / others
This line is largely speculative - but should follow the idea of user scripts & adblock repos.
This will start as a set of local canonicalization transformations.

Subsystem: Browsing
1. Request [QueryCID, Request]_encrypted to PeerID of Exit
2. Optional Location to provide response
3. Receive back Attestation
4. Response&attestation pushed to storage location as directed. (degraded option is to send directly back to client)
    Note: exit likely should parse enough html to push an archive with expected subresources as well and try to skip round-trips

Notes:
Exit's work is probably gated with privacy pass

Subsystem: Data Repos
1. Store on behalf of user, should see if something like storacha can serve this role
2. At the end of the day, a blockstore that holds content addressed data - ipfs/filecoin/bsky pds could all potentially work
