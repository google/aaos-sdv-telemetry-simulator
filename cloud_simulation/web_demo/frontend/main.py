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


from flask import Flask, send_from_directory
import os

# Initialize the Flask app
app = Flask(__name__, static_folder="build/web")

# Route for serving index.html
@app.route("/")
@app.route("/<path:path>")
def serve_flutter(path=""):
    if path != "" and os.path.exists(f"{app.static_folder}/{path}"):
        # Return requested static file
        return send_from_directory(app.static_folder, path)
    else:
        # Fallback to index.html for SPA routing
        return send_from_directory(app.static_folder, "index.html")

if __name__ == "__main__":
    # Start the Flask development server locally for testing
    app.run(host="0.0.0.0", port=8080)