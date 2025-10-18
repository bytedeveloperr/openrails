This directory holds the GitHub personal access token used for Docker builds.

Place your token (with at least `repo` scope) in a file named `gh_token` before
running `docker buildx` locally. The CI workflow writes this file automatically
from the `GH_TOKEN` repository secret and deletes it after use.
