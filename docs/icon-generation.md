# Application icon

Generated with the built-in `image_gen` tool on 2026-10-22. The original `resources/icon.png` is a 1254 × 1254 RGBA PNG used by the native header and Windows.

On macOS, MyGo compiles `assets/macos/GistManager.icon` with Xcode 26 or later. The document uses the blue gradient as the icon's background and the two code cards as foreground layers, so the system applies a single rounded shape. The bundle contains `Assets.car` for modern macOS and `AppIcon.icns` for older versions. With only Command Line Tools installed, MyGo falls back to an ICNS generated from the original PNG; the release workflow requires the compiled catalog.

Prompt:

```text
Use case: logo-brand. Asset type: production desktop app icon for a native GitHub Gist manager, square 1024x1024 PNG with genuine transparent background. Create ONE polished, original icon, front-facing and centered. A confident cobalt-blue rounded-square tile, with a subtle azure edge and restrained dimensional depth. Inside, two overlapping compact code snippet cards form a single bold silhouette: the front card is off-white with a large, exceptionally clear dark-blue angle-bracket slash code symbol, and the rear card peeks out in cyan. Preserve generous negative space and chunky geometric details that remain clear at 16, 32 and 48 pixels. Palette must fit the existing app: royal blue #2563EB, deeper blue #1D4ED8, cyan #38BDF8, deep navy #0F172A, white. Clean professional developer-tool aesthetic, crisp smooth edges, balanced optical weight, gently rounded corners, no excessive gloss. The icon tile occupies about 88 percent of the canvas, with transparent pixels around the rounded outer silhouette. No background scene, no ground plane, no drop shadow outside the icon, no letters, no app name, no watermark, no grid, no alternate concepts, no mockup. Only the single finished application icon.
```
