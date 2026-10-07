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
  version "1.15.0"
  license "MIT"

  if OS.mac? && Hardware::CPU.arm?
    url "https://github.com/sky-jiangcheng/repo-nest/releases/download/v#{version}/reponest-mcp-darwin-arm64.tar.gz"
    sha256 "3cd3140f5cb077fdc618403fbbee4d8967999ee465c13a8016cd97bd23ba116c"
  elsif OS.mac? && Hardware::CPU.is_64_bit?
    url "https://github.com/sky-jiangcheng/repo-nest/releases/download/v#{version}/reponest-mcp-darwin-amd64.tar.gz"
    sha256 "5f3ef4fd7bdfc7651fbf214fdadfd74793728d3a0b80df60a6e08d773b1cf4ef"
  else
    url "https://github.com/sky-jiangcheng/repo-nest/releases/download/v#{version}/reponest-mcp-linux-amd64.tar.gz"
    sha256 "8f25b012fdbedea4e958657b11efcede5567cc322f9e9aa6c3e38cce030db21f"
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
