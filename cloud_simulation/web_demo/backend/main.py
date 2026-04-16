# Copyright 2025 Google LLC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

import os
from flask import Flask, request, jsonify, Response
from flask_cors import CORS
import requests
from google.auth.transport import requests as google_requests
from google.oauth2 import id_token
from google.cloud import storage

# Environment-specific configurations - Will raise KeyError if not set
CORS_ORIGIN = os.environ["CORS_ORIGIN"]
CLOUD_FUNCTION_SCHEDULE = os.environ["CLOUD_FUNCTION_SCHEDULE"]
CLOUD_FUNCTION_DELETE = os.environ["CLOUD_FUNCTION_DELETE"]
BACKEND_URL = os.environ["BACKEND_URL"]
OAUTH_CLIENT_ID = os.environ["OAUTH_CLIENT_ID"]
BUCKET_NAME = os.environ["BUCKET_NAME"]
PROJECT_ID = os.environ["PROJECT_ID"]

BACKEND_ALLOWED_AUDIENCE = [OAUTH_CLIENT_ID, BACKEND_URL]

# Initialize Flask app
app = Flask(__name__)

# Enable CORS for specific origin
CORS(app, origins=[CORS_ORIGIN], supports_credentials=True)

### Helper Functions

def verify_token(auth_header):
    """
    Verifies the incoming ID token from the frontend.
    """
    if not auth_header or not auth_header.startswith("Bearer "):
        raise ValueError("Missing or invalid Authorization header")
    token = auth_header.split(" ")[1]
    auth_request = google_requests.Request()
    try:
        token_info = id_token.verify_oauth2_token(token, auth_request, BACKEND_ALLOWED_AUDIENCE)
        print(f"Token verified for: {token_info.get('email', 'unknown')}")
        return token_info
    except ValueError as e:
        print(f"Token verification failed: {e}")
        raise ValueError("Invalid ID token")

@app.before_request
def log_request_info():
    print(f"Incoming request: {request.method} {request.url}")
    print(f"Request headers: {request.headers}")
    print(f"Request body: {request.data}")

@app.after_request
def add_cors_headers(response):
    """
    Add necessary CORS headers to all responses.
    """
    response.headers["Access-Control-Allow-Origin"] = CORS_ORIGIN
    response.headers["Access-Control-Allow-Methods"] = "POST, GET, OPTIONS"
    response.headers["Access-Control-Allow-Headers"] = "Content-Type, Authorization, X-Goog-Authuser"
    response.headers["Access-Control-Allow-Credentials"] = "true"
    return response

### Simulation API Routes

@app.route("/api/scheduleSimulation", methods=["POST", "OPTIONS"])
def schedule_simulation():
    if request.method == "OPTIONS":
        # Handle preflight requests
        print("OPTIONS request for /scheduleSimulation")
        return Response(status=204)  # No content
    try:
        auth_header = request.headers.get("Authorization", "")
        token_info = verify_token(auth_header)
        print(f"Received scheduleSimulation request from: {token_info.get('email', 'unknown')}")

        body = request.json
        if not body:
            return jsonify({"error": "Invalid or missing request body"}), 400

        auth_request = google_requests.Request()
        id_token_function = id_token.fetch_id_token(auth_request, CLOUD_FUNCTION_SCHEDULE)

        response = requests.post(
            CLOUD_FUNCTION_SCHEDULE,
            headers={
                "Authorization": f"Bearer {id_token_function}",
                "Content-Type": "application/json",
            },
            json=body,
        )
        return (response.text, response.status_code, response.headers.items())
    except ValueError as e:
        print(f"Authorization failed: {e}")
        return jsonify({"error": "Unauthorized"}), 401
    except Exception as e:
        print(f"Error scheduling simulation: {e}")
        return jsonify({"error": "Internal server error"}), 500

@app.route("/api/cancelSimulation", methods=["POST", "OPTIONS"])
def cancel_simulation():
    if request.method == "OPTIONS":
        # Handle preflight requests
        print("OPTIONS request for /cancelSimulation")
        return Response(status=204)  # No content
    try:
        auth_header = request.headers.get("Authorization", "")
        token_info = verify_token(auth_header)
        print(f"Received cancelSimulation request from: {token_info.get('email', 'unknown')}")

        body = request.json
        if not body:
            return jsonify({"error": "Invalid or missing request body"}), 400

        auth_request = google_requests.Request()
        id_token_function = id_token.fetch_id_token(auth_request, CLOUD_FUNCTION_DELETE)

        response = requests.post(
            CLOUD_FUNCTION_DELETE,
            headers={
                "Authorization": f"Bearer {id_token_function}",
                "Content-Type": "application/json",
            },
            json=body,
        )
        return (response.text, response.status_code, response.headers.items())
    except ValueError as e:
        print(f"Authorization failed: {e}")
        return jsonify({"error": "Unauthorized"}), 401
    except Exception as e:
        print(f"Error cancelling simulation: {e}")
        return jsonify({"error": "Internal server error"}), 500


@app.route("/api/getFileContent", methods=["POST", "OPTIONS"])
def get_file_content():
    if request.method == "OPTIONS":
        # Handle CORS preflight request.
        print("OPTIONS request for /getFileContent")
        return Response(status=204)  # No content for preflight

    try:
        # Verify the incoming token from the frontend
        auth_header = request.headers.get("Authorization", "")
        token_info = verify_token(auth_header)

        # Parse the request body
        body = request.json
        if not body or "filePath" not in body:
            return jsonify({"error": "Missing 'filePath' in request body"}), 400

        file_path = body["filePath"]
        print(f"Fetching file content for {file_path}, requested by: {token_info.get('email')}")

        # Create a Cloud Storage client
        storage_client = storage.Client()

        # Access the bucket and blob (file)
        bucket = storage_client.bucket(BUCKET_NAME)
        blob = bucket.blob(file_path)

        if not blob.exists():
            return jsonify({"error": f"File not found: {file_path}"}), 404

        # Download file content
        content = blob.download_as_text()
        print(f"Successfully fetched file content for {file_path}")
        return Response(content, mimetype="text/plain")  # Send back plain text content

    except ValueError as e:
        print(f"Authorization failed: {e}")
        return jsonify({"error": "Unauthorized"}), 401
    except Exception as e:
        print(f"Error fetching file content: {e}")
        return jsonify({"error": "Internal server error"}), 500

### Error Handling for Missing Routes

@app.errorhandler(404)
def page_not_found(e):
    print(f"Unmatched route: {request.path}")
    return jsonify({"error": "Route not found"}), 404


if __name__ == "__main__":
    # Use for local debugging; in production, use gunicorn with App Engine.
    app.run(host="0.0.0.0", port=8080)
