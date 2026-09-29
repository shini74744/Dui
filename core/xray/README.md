# DUI Xray vx-26.1

Based on official XTLS/Xray-core v26.9.9. Complete reproducible modified source, platform binaries and SHA256 checksums are published together.

- Optional allowInsecure is restored, disabled by default, with a warning. Explicit certificate pin/name checks remain effective.
- Real per-user upload and download rate limits, shared across all connections, are enforced independently. DUI stores speedLimitMbps; 300 means 300 Mbps upload and 300 Mbps download. Zero is unlimited. Legacy speedLimit KiB/s values are converted without changing units silently.
- Reserved protocol user levels (high bit plus bytes/second) carry DUI rates through static configuration and the existing user API. Normal policy levels are unchanged. Reserved rate levels inherit policy level 0 timeouts; uplinkOnly/downlinkOnly remain seconds.
- SS2022 user levels accept uint32, and the panel uses the new SS2022 Account protobuf. Limited Vision traffic cannot bypass the limiter via splice.
- Retains the DUI CLOSE-WAIT fix: propagate write EOF while allowing the response direction to finish; full close interrupts blocked reads and ends the activity timer.
- Panel compatibility uses current TLS certificate field names, removes retired echForceQuery from generated configuration, and preserves explicit trustedXForwardedFor proxy settings. Only add trusted reverse proxy addresses; the empty default trusts none.

Tests cover limiter sharing and independent directions, cancellation, UDP buffer metadata, real multi-connection VLESS, Mux, REALITY/Vision and SS2022 TCP throughput plus SS2022 UDP packet integrity/throughput, TLS strict/insecure/pinned handshakes, and connection close/half-close. Per-platform compilation does not imply a live network test on every OS.

This release does not incorporate post-v26.9.9 upstream commits such as the later SS2022 rewrite. An RSS reduction is not guaranteed.
