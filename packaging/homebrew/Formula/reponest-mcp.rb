# RepoNest MCP server (Linux)
#
# The desktop app has no Linux installer artifact (it ships as a bare tarball),
# but the MCP server is a pure Go / zero-CGO binary and installs cleanly, so the
# Formula covers MCP on Linux rather than the GUI app.
#
# Version is stamped by scripts/update-manifests.sh from wails.json
# (info.productVersion) — never edit it by hand.
class ReponestMcp < Formula
  desc "MCP stdio server exposing the RepoNest local knowledge base to AI agents"
  homepage "https://github.com/sky-jiangcheng/repo-nest"
  version "1.16.0"
  license "MIT"

  if OS.mac? && Hardware::CPU.arm?
    url "https://github.com/sky-jiangcheng/repo-nest/releases/download/v#{version}/reponest-mcp-darwin-arm64.tar.gz"
    sha256 "4888d08429f909d25520e293cb6febc781ef2972c698ababb140bea8852d8cae"
  elsif OS.mac? && Hardware::CPU.is_64_bit?
    url "https://github.com/sky-jiangcheng/repo-nest/releases/download/v#{version}/reponest-mcp-darwin-amd64.tar.gz"
    sha256 "2d14bc74ac19dbfa1bb4978c0e169a09501337088f4eb2e855ee6c46f1daee01"
  else
    url "https://github.com/sky-jiangcheng/repo-nest/releases/download/v#{version}/reponest-mcp-linux-amd64.tar.gz"
    sha256 "616eed0234e31a32e6cee1a8c7f4756ee4ea7da23e31483275a213ce68354034"
  end

  def install
    bin.install "reponest-mcp"
  end

  # The binary is a stdio MCP server: running it with no stdin makes it block
  # on the JSON-RPC loop, so it cannot be executed as a test. Assert the file
  # landed and is executable instead of pretending to run it.
  test do
    assert_path_exists bin/"reponest-mcp"
    assert_predicate bin/"reponest-mcp", :executable?
  end
end
