# argo-deployments
Argocd deployments for our cluster.

This is a monorepo, containing agro cd deployment charts and images alongside their manifests.

to build an image you need to specify it and tag it:  
`git tag we-quota-checker/v0.1.0`  
then push it to its respective tag  
`git push origin we-quota-checker/v0.1.0`  

Available deployments:
- twg: wireguard + wstunnel (needs more work)
- we-quota-checker: open telemetry code for prometheus to scrape