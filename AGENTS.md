# GSNote development

- Run the development bot from the repository root in a named Screen session:
  `screen -dmS gsnote make air`
- Inspect current output with:
  `screen -S gsnote -X hardcopy /tmp/gsnote-screen.txt`
  then read `/tmp/gsnote-screen.txt`.
- `make air` runs Air and the bot as its child process. A stale `running...` line does not prove the latest build started.
- Do not send Ctrl-C through `screen -X stuff` merely to inspect logs; it can terminate the Screen session and bot. Capture logs and process state first.
- Before restarting a live bot, check for duplicate Telegram pollers and preserve the existing DB, OGG files, and transcripts.
- Stop intentionally with `screen -S gsnote -X quit`.
