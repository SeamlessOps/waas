import json
import argparse
from datetime import datetime
import os
from pathlib import Path

# Environment Variables
METRICS_FILE = os.getenv('METRICS_FILE', 'waas-metrics/data/deployments.json')

def update_metrics(args):
    try:
        # Ensure the directory exists
        metrics_path = Path(METRICS_FILE)
        metrics_path.parent.mkdir(parents=True, exist_ok=True)
        
        # Load existing data
        try:
            with open(METRICS_FILE, 'r') as f:
                data = json.load(f)
        except FileNotFoundError:
            data = {"deployments": []}
        except json.JSONDecodeError as e:
            print(f"⚠️ Warning: Invalid JSON in {METRICS_FILE}, creating new file. Error: {e}")
            data = {"deployments": []}
        
        # Create new deployment entry
        # Convert duration safely, default to 0
        try:
            duration_seconds = float(args.duration) if args.duration else 0.0
        except ValueError:
            duration_seconds = 0.0

        deployment = {
            "timestamp": datetime.utcnow().isoformat(),
            "environment": args.environment,
            "service": args.service,
            "service_type": args.service_type,
            "status": args.status,
            "duration_seconds": duration_seconds,
            "commit_hash": args.commit_hash,
            "commit_author": os.getenv('COMMIT_AUTHOR', ''),
            "commit_message": os.getenv('COMMIT_MESSAGE', ''),
            "actions_url": args.actions_url,
            "image_tag": args.image_tag
        }
        
        # Add to beginning of list (most recent first)
        data["deployments"].insert(0, deployment)
        data["deployments"] = data["deployments"][:100]  # Keep recent ones

        # Write back to file
        with open(METRICS_FILE, 'w') as f:
            json.dump(data, f, indent=2)
        
        print(f"✅ Updated metrics for {args.service} in {args.environment}")
        print(f"📊 Total deployments: {len(data['deployments'])}")
        print(f"💾 Saved to: {METRICS_FILE}")
        
    except Exception as e:
        print(f"❌ Error updating metrics: {e}")
        raise

if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument('--environment', required=True)
    parser.add_argument('--service', required=True)
    parser.add_argument('--service_type', required=True)
    parser.add_argument('--status', required=True)
    parser.add_argument('--duration', required=False)
    parser.add_argument('--commit_hash', required=True)
    parser.add_argument('--actions_url', required=True)
    parser.add_argument('--image_tag', required=True)
    
    args = parser.parse_args()
    update_metrics(args)