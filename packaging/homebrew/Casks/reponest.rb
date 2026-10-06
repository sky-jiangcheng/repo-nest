# RepoNest desktop app (macOS)
#
# Version is stamped by scripts/update-manifests.sh from wails.json
# (info.productVersion) — never edit it by hand.
#
# sha256 must be filled after each release is published; Homebrew hard-fails on
# a mismatch, which is exactly what we want from a placeholder.
cask "reponest" do
  version "1.14.4"
  name "RepoNest"
  desc "Local-first code project context base"
  homepage "https://github.com/sky-jiangcheng/repo-nest"
  license "MIT"

  on_arm do
    url "https://github.com/sky-jiangcheng/repo-nest/releases/download/v#{version}/reponest-darwin-arm64.dmg"
    sha256 "c286cca98ded813e592b080d9a549901b31f39d05e89cf8ae0c0784c7b8c953e"
  end

  on_intel do
    url "https://github.com/sky-jiangcheng/repo-nest/releases/download/v#{version}/reponest-darwin-amd64.dmg"
    sha256 "dbb0016f968b0efcd90a29422f0ec57f27ce661e5f7f4bc74b326d3fda58ad09"
  end

  app "RepoNest.app"

  # Application Support and Logs paths come from internal/platform; they are
  # where GetDbPath() and GetLogPath() point on macOS. A bundle-identifier
  # preference is deliberately not listed here: wails.json does not set one, so
  # listing a guess would send `brew uninstall --zap` at a file that may not
  # exist. Add it only once the identifier is pinned in wails.json.
  zap trash: [
    "~/Library/Application Support/reponest",
    "~/Library/Logs/reponest.log",
  ]
end
