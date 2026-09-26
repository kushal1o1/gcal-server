package server

import "net/http"

// handleRoot serves the branded HTML landing page at "/".
func handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(landingHTML))
}

const landingHTML = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1, viewport-fit=cover">
<title>gcal · voice reminders for google calendar</title>
<style>
  :root {
    --bg: #05070a;
    --bg-radial: #0a1420;
    --scan: rgba(137, 180, 250, 0.035);
    --ink: #cdebff;
    --dim: #5b7a8c;
    --blue: #89B4FA;
    --pink: #F38BA8;
    --hair: #16232b;
    --glow-a: rgba(137,180,250,0.85);
    --glow-b: rgba(137,180,250,0.35);
    --logo-mark: #05070a;
    color-scheme: dark;
  }
  @media (prefers-color-scheme: light) {
    :root {
      --bg: #eef2f7;
      --bg-radial: #ffffff;
      --scan: rgba(50, 90, 140, 0.03);
      --ink: #1c2b3a;
      --dim: #6b7c8c;
      --blue: #2f6fd6;
      --pink: #d6467c;
      --hair: #d3dce4;
      --glow-a: rgba(47,111,214,0.25);
      --glow-b: rgba(47,111,214,0.12);
      --logo-mark: #ffffff;
      color-scheme: light;
    }
  }
  * { box-sizing: border-box; }
  html, body { margin: 0; height: 100%; }
  body {
    background:
      repeating-linear-gradient(0deg, var(--scan) 0px, var(--scan) 1px, transparent 1px, transparent 3px),
      radial-gradient(ellipse at 50% 15%, var(--bg-radial) 0%, var(--bg) 60%);
    color: var(--ink);
    font-family: "SFMono-Regular", ui-monospace, Menlo, Consolas, "Liberation Mono", monospace;
    min-height: 100%;
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 32px 16px;
    padding-top: max(32px, env(safe-area-inset-top));
    padding-bottom: max(32px, env(safe-area-inset-bottom));
  }

  main {
    width: 100%;
    max-width: 640px;
    text-align: center;
  }

  .logo-wrap {
    display: flex;
    justify-content: center;
    margin-bottom: 18px;
  }
  .logo {
    width: 64px;
    height: 64px;
    filter: drop-shadow(0 0 18px rgba(137,180,250,0.45));
  }

  .ascii {
    margin: 0;
    display: inline-block;
    font-size: clamp(9px, 3.6vw, 20px);
    line-height: 1.15;
    letter-spacing: 0.05em;
    color: var(--blue);
    text-shadow: 0 0 6px var(--glow-a), 0 0 24px var(--glow-b);
    white-space: pre;
    user-select: all;
  }

  .caret {
    display: inline-block;
    width: 0.55em;
    height: 1em;
    background: var(--pink);
    vertical-align: -0.15em;
    margin-left: 4px;
    box-shadow: 0 0 10px rgba(243,139,168,0.8);
    animation: blink 1.1s steps(1) infinite;
  }
  @keyframes blink { 50% { opacity: 0; } }

  .tagline {
    color: var(--dim);
    font-size: 13.5px;
    letter-spacing: 0.02em;
    margin: 24px 0 6px;
    max-width: 34ch;
    margin-inline: auto;
  }

  footer {
    margin-top: 28px;
    color: var(--dim);
    font-size: 11.5px;
  }
  footer .prompt { color: var(--blue); }
  footer a {
    color: var(--pink);
    text-decoration: none;
    border-bottom: 1px dashed var(--pink);
  }
  footer a:hover, footer a:focus-visible { opacity: 0.8; }

  @media (prefers-reduced-motion: reduce) {
    .caret { animation: none; opacity: 1; }
  }
</style>
</head>
<body>
  <main>
    <div class="logo-wrap">
      <svg class="logo" viewBox="0 0 512 512" aria-hidden="true">
        <rect x="86" y="86" width="340" height="340" rx="64" fill="#89B4FA"/>
        <rect x="124" y="112" width="112" height="48" rx="24" fill="var(--logo-mark)"/>
        <circle cx="347" cy="347" r="39" fill="#F38BA8"/>
        <text x="266" y="272" font-size="300" font-weight="900" fill="var(--logo-mark)"
              text-anchor="middle" dominant-baseline="middle" font-family="ui-sans-serif, system-ui, sans-serif">G</text>
      </svg>
    </div>

    <pre class="ascii">
 ███   ███   ███  █    
█   █ █   █ █   █ █    
█     █     █   █ █    
█ ██  █     █████ █    
█   █ █     █   █ █    
█   █ █   █ █   █ █    
 ███   ███  █   █ █████<span class="caret"></span></pre>

    <p class="tagline">talk to it, it reminds you later.</p>

    <footer><span class="prompt">$</span> made with go by <a href="https://github.com/kushal1o1" target="_blank" rel="noopener">kushal1o1</a></footer>
  </main>
</body>
</html>`
