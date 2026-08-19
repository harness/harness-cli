# Locust checkout journey.
#
# The host, user count, spawn rate and run time come from the load test
# definition, so this file sets none of them. Test variables are passed as
# environment variables, so a definition carrying `baseUrl` is read with
# os.environ.

import json
import os
import random

from locust import HttpUser, between, task


class CheckoutUser(HttpUser):
    # min_wait/max_wait equivalent. The definition's tunables control load
    # volume; this only shapes the pause between a single user's requests.
    wait_time = between(1, 3)

    def on_start(self):
        """Authenticate once per simulated user and keep the token."""
        credentials = {
            "user": os.environ.get("PERF_USER", "perf"),
            "password": os.environ.get("PERF_PASSWORD", ""),
        }
        with self.client.post(
            "/v1/auth/login",
            json=credentials,
            catch_response=True,
            name="login",
        ) as response:
            if response.status_code != 200:
                response.failure(f"login failed: {response.status_code}")
                return
            token = response.json().get("data", {}).get("accessToken")
            self.client.headers.update({"Authorization": f"Bearer {token}"})

    @task(3)
    def browse_products(self):
        self.client.get("/v1/products?limit=20", name="products")

    @task(1)
    def checkout(self):
        payload = {"sku": f"SKU-{random.randint(1000, 1099)}", "quantity": 1}
        with self.client.post(
            "/v1/cart/checkout",
            data=json.dumps(payload),
            headers={"Content-Type": "application/json"},
            catch_response=True,
            name="checkout",
        ) as response:
            if response.status_code >= 400:
                response.failure(f"checkout failed: {response.status_code}")
