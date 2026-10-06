# RepoNest MCP server (macOS)
#
# The MCP stdio server is the AI execution interface for RepoNest — the desktop
# app is not required to use it, it reads the same local SQLite database.
#
# Version is stamped by scripts/update-manifests.sh from wails.json
# (info.productVersion) — never edit it by hand.
cask "reponest-mcp" do
  version "1.14.3"
  name "RepoNest MCP Server"
  desc "MCP stdio server exposing the RepoNest local knowledge base to AI agents"
  homepage "https://github.com/sky-jiangcheng/repo-nest"
  license "MIT"

  on_arm do
    url "https://github.com/sky-jiangcheng/repo-nest/releases/download/v#{version}/reponest-mcp-darwin-arm64.tar.gz"
    sha256 "2bcec04c2244f0464635b793e8d47ad9c3e453b8b58c2164472c0fede851a021"
  end

  on_intel do
    url "https://github.com/sky-jiangcheng/repo-nest/releases/download/v#{version}/reponest-mcp-darwin-amd64.tar.gz"
    sha256 "08923acaa5e706a52a58a449d7bf65e56864f8e8ab14ac74d88c8bc335176f2b"
  end

  # The tarball stages a single bare `reponest-mcp` at its root; `binary` links
  # it into $(brew --prefix)/bin so `which reponest-mcp` works for
  # `claude mcp add reponest -- $(which reponest-mcp)`.
  binary "reponest-mcp"
end
