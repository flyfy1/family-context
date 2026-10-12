# Integ-Chat naming decision

Goal: apply Yangyu’s exact official product name Integ-Chat in Family Daily’s public product switcher.
Change: update only the shared navigation dependency from UI 0.5.0 to 0.5.1.
Source baseline is the exact live Family Daily release 83d416a. The frontend deploys through its existing GitHub Pages workflow.
Validation and production results will be appended here after publication.

Validation: npm run check passed (existing Node tests and Vite build). Source commit 3c4b28e88b414db224e25b40c04cea99dfe3c4ee was pushed through the existing main-push GitHub Pages workflow.
Live curl: family.integ.life returned 200, its loaded JavaScript has Integ-Chat and no Integ.Chat or LLM Room; build-version.json reports the exact source commit, dirty=false, uiVersion=0.5.1. Evidence: ~/integ-life-inspection/reports/evidence-20261012-decisions/additional-naming-audit.json.
Life worklog Note ec261300-c43e-4316-a566-b939ef6c7775 is retained in the local cache. life note sync could not run because INTEGLIFE_API_TOKEN is not set; synced remains false.
