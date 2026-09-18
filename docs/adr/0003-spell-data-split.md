# Spell flags and timing are generated; damage numbers stay in Go

Spell flags and timing are generated from server data: damage class, school, cast time, GCD, cooldown,
duration, stacks, and the binary and defense attributes. Damage numbers and coefficients stay hardcoded
in Go next to the logic that uses them, and a test fails on any mismatch with server data.
