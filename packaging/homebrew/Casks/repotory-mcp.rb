# Repotory MCP server (macOS)
#
# The MCP stdio server is the AI execution interface for Repotory — the desktop
# app is not required to use it, it reads the same local SQLite database.
#
# Version is stamped by scripts/update-manifests.sh from wails.json
# (info.productVersion) — never edit it by hand.
cask "repotory-mcp" do
  version "2.0.0"
  name "Repotory MCP Server"
  desc "MCP stdio server exposing the Repotory local knowledge base to AI agents"
  homepage "https://github.com/sky-jiangcheng/repo-nest"
  license "MIT"

  on_arm do
    url "https://github.com/sky-jiangcheng/repo-nest/releases/download/v#{version}/repotory-mcp-darwin-arm64.tar.gz"
    sha256 "b7fa4bfd6cc49bbfed3916b88f8de6e505ebaede6a7c6bae5eb7e6e8f62fe3b7"
  end

  on_intel do
    url "https://github.com/sky-jiangcheng/repo-nest/releases/download/v#{version}/repotory-mcp-darwin-amd64.tar.gz"
    sha256 "75682bc0ec892ffdbab31fbbea0cea89536baa10063128332283aac83f730bdd"
  end

  # The tarball stages a single bare `repotory-mcp` at its root; `binary` links
  # it into $(brew --prefix)/bin so `which repotory-mcp` works for
  # `claude mcp add repotory -- $(which repotory-mcp)`.
  binary "repotory-mcp"
end
