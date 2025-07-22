# Adding New Command Types

This repository uses the `CommandProcessor` interface to decouple fetching logic from CLI commands. To introduce a new command, follow these steps:

1. **Create a processor**
   - Implement a new struct that holds any required clients or configuration.
   - Provide `FetchData()` to perform API calls and `GetName()` for user facing messages.

2. **Add a Cobra command**
   - In `init()` or a dedicated setup function, create a `cobra.Command` that instantiates the processor and runs it through `CommandRunner`.

3. **Register the command**
   - Add the new command to `rootCmd` so it becomes part of the CLI.

See the `CommandProcessor` interface in `main.go` for additional details.
