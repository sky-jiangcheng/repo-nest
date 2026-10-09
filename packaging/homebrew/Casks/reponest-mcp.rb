# RepoNest MCP server (macOS)
#
# The MCP stdio server is the AI execution interface for RepoNest — the desktop
# app is not required to use it, it reads the same local SQLite database.
#
# Version is stamped by scripts/update-manifests.sh from wails.json
# (info.productVersion) — never edit it by hand.
cask "reponest-mcp" do
  version "1.16.1"
  name "RepoNest MCP Server"
  desc "MCP stdio server exposing the RepoNest local knowledge base to AI agents"
  homepage "https://github.com/sky-jiangcheng/repo-nest"
  license "MIT"

  on_arm do
    url "https://github.com/sky-jiangcheng/repo-nest/releases/download/v#{version}/reponest-mcp-darwin-arm64.tar.gz"
    sha256 "67ba6a697925bc15ad6cc77f824e2b71f7d3871420a124e9e1635a510e2aa318"
  end

  on_intel do
    url "https://github.com/sky-jiangcheng/repo-nest/releases/download/v#{version}/reponest-mcp-darwin-amd64.tar.gz"
    sha256 "f319ac581f5f05f3a07d5602a74a938a81843cdb4f659774ec977b80a8e385ae"
  end

  # The tarball stages a single bare `reponest-mcp` at its root; `binary` links
  # it into $(brew --prefix)/bin so `which reponest-mcp` works for
  # `claude mcp add reponest -- $(which reponest-mcp)`.
  binary "reponest-mcp"
end
