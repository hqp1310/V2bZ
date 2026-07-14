import urllib.request
import json
import zipfile
import io

url = "https://api.github.com/repos/hqp1310/zicnode/actions/runs?per_page=1"
req = urllib.request.Request(url)
with urllib.request.urlopen(req) as response:
    data = json.loads(response.read().decode())
    
run_id = data['workflow_runs'][0]['id']
print(f"Latest run ID: {run_id}")

log_url = f"https://api.github.com/repos/hqp1310/zicnode/actions/runs/{run_id}/logs"
req = urllib.request.Request(log_url)
try:
    with urllib.request.urlopen(req) as response:
        with zipfile.ZipFile(io.BytesIO(response.read())) as z:
            for filename in z.namelist():
                if "Build ZicNode" in filename or "Setup Go" in filename:
                    print(f"--- {filename} ---")
                    print(z.read(filename).decode())
except Exception as e:
    print(f"Failed to fetch logs: {e}")
