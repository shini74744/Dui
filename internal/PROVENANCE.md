# Protocol helpers

The `tuic`, `mtproto`, `amneziawg`, `amneziawgnet` and `util/wireguard`
packages are adapted from MHSanaei/3x-ui commit
`8c023d13dc9d01a72bf8be6fa409c6af392b8088` (GPL-3.0).
Source: https://github.com/MHSanaei/3x-ui/tree/8c023d13dc9d01a72bf8be6fa409c6af392b8088/internal

DUI adaptations retain upstream tests and add integration tests. Process ownership,
binary paths, validation, runtime coordination and feature capabilities are checked
against DUI rather than assumed equivalent to the upstream panel.
