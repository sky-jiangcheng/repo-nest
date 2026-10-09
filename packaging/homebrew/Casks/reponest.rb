# RepoNest desktop app (macOS)
#
# Version is stamped by scripts/update-manifests.sh from wails.json
# (info.productVersion) — never edit it by hand.
#
# sha256 must be filled after each release is published; Homebrew hard-fails on
# a mismatch, which is exactly what we want from a placeholder.
cask "reponest" do
  version "1.16.2"
  name "RepoNest"
  desc "Local-first code project context base"
  homepage "https://github.com/sky-jiangcheng/repo-nest"
  license "MIT"

  on_arm do
    url "https://github.com/sky-jiangcheng/repo-nest/releases/download/v#{version}/reponest-darwin-arm64.dmg"
    sha256 "e25a4601f3949886c2ed7131c1c6bf4781a639a92730918bf5f76fd9e3be8a2e"
  end

  on_intel do
    url "https://github.com/sky-jiangcheng/repo-nest/releases/download/v#{version}/reponest-darwin-amd64.dmg"
    sha256 "feca66acc6ab6770879537ac45a50ee82c7e4affbfd15b21b8a7bfb7c4b364b6"
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
