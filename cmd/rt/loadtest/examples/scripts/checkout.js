// k6 checkout journey.
//
// Tunables set in the load test definition (targetUsers, durationSeconds,
// rampUpTimeSec, ...) are applied by the runner, so this script does not
// declare its own `options.stages`. Declaring them here would override the
// values the CLI sends and make --target-users have no effect.
//
// Test variables reach the script as environment variables, so a definition
// carrying `baseUrl` is read with __ENV.baseUrl.

import http from 'k6/http';
import { check, group, sleep } from 'k6';

const baseUrl = __ENV.baseUrl || 'https://staging.example.com';

export default function () {
  group('browse', function () {
    const listing = http.get(`${baseUrl}/v1/products?limit=20`, {
      tags: { endpoint: 'products' },
    });
    check(listing, {
      'products 200': (r) => r.status === 200,
    });
  });

  sleep(1);

  group('checkout', function () {
    const payload = JSON.stringify({ sku: 'SKU-1001', quantity: 1 });
    const order = http.post(`${baseUrl}/v1/cart/checkout`, payload, {
      headers: { 'Content-Type': 'application/json' },
      tags: { endpoint: 'checkout' },
    });
    check(order, {
      'checkout 2xx': (r) => r.status >= 200 && r.status < 300,
      'order id returned': (r) => r.json('data.orderId') !== undefined,
    });
  });

  sleep(2);
}
