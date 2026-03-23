Create a new GitHub release with a tag.

## Steps

1. Fetch latest tags: `git -C $CWD fetch --tags`
2. List recent tags to determine the next version: `git -C $CWD tag --sort=-v:refname | head -10`
3. If `$ARGUMENTS` is provided, use it as the version tag. Otherwise, suggest the next semantic version (bump patch by default) and ask the user to confirm or specify a different version.
4. Check that the working tree is clean (`git -C $CWD status --porcelain`). If not, warn the user and stop.
5. Generate release notes from commits since the last tag: `git -C $CWD log <last-tag>..HEAD --oneline --no-decorate`
6. Show the release notes to the user and ask for confirmation.
7. Create an annotated tag: `git -C $CWD tag -a <version> -m "Release <version>"`
8. Push the tag: `git -C $CWD push origin <version>`
9. Create a GitHub release using: `gh release create <version> --repo velom/HomeLibraryBot --title "<version>" --notes "<release-notes>" --latest`
10. Show the user the release URL.
