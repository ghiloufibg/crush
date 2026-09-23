List Kubernetes pods via `kubectl get pods`. Returns namespace, name, phase, and total container restarts for each pod, sorted by namespace then name. Defaults to the current kubeconfig context's namespace; set all_namespaces to list across the whole cluster.

Results are capped at 50 by default (see max_results). To narrow scope instead of raising the cap, prefer label_selector (filtered by kubectl itself, cheapest) or name_contains (filtered after fetching). If the response was truncated, it says how many more pods matched so you know the cluster wasn't fully listed.
