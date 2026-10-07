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
    sha256 "93881da46f5f30cb59df349179e180bc820b81b8ed91f44dad4785d826b0e553"
  elsif OS.mac? && Hardware::CPU.is_64_bit?
    url "https://github.com/sky-jiangcheng/repo-nest/releases/download/v#{version}/reponest-mcp-darwin-amd64.tar.gz"
    sha256 "62acb34b5312495d8337d5f0c41131083074575acf50581df4bde8705abe04ab"
  else
    url "https://github.com/sky-jiangcheng/repo-nest/releases/download/v#{version}/reponest-mcp-linux-amd64.tar.gz"
    sha256 "e593bba19157b90ff6b837ac737e65eb13e2b64596954da94d44daa2b0c91029"
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
