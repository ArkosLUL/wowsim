# Model the server exactly, as a permanent fork

The sim reproduces the user's AzerothCore server, even where the server deviates from retail 3.3.5a.
Each deviation is logged in the parity INVESTIGATION, so the server can be patched and the sim updated
later. Classic rules are replaced in place, with no ruleset toggle, so upstream wowsims/wotlk is never
merged again. With a single local user, share links and saved settings need no migrations.
