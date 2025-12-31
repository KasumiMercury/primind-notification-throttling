import { check, sleep } from "k6";
import http from "k6/http";
import { Counter, Trend } from "k6/metrics";
import { config } from "./config.js";
import { STRICT_THRESHOLD_SECONDS } from "./constants.js";
import {
    addMinutes,
    formatRFC3339,
    generateRunID,
    parseDate,
} from "./helpers.js";

const packetsProcessed = new Counter("packets_processed");
const packetsSuccess = new Counter("packets_success");
const packetsFailed = new Counter("packets_failed");
const processingDuration = new Trend("processing_duration_ms");

export const options = {
    scenarios: {
        throttle_test: {
            executor: "shared-iterations",
            vus: 1,
            iterations: config.iterations,
            maxDuration: config.maxDuration,
        },
    },
    thresholds: {
        http_req_failed: ["rate<0.01"],
        http_req_duration: ["p(95)<10000"],
    },
};

function logHeader(title) {
    const line = "=".repeat(50);
    console.log(`\n${line}`);
    console.log(`  ${title}`);
    console.log(line);
}

function logBuckets(buckets) {
    let total = 0;
    buckets.forEach((b, i) => {
        const lane =
            b.slide_window_width <= STRICT_THRESHOLD_SECONDS
                ? "strict"
                : "loose";
        const timeRange = `${b.start_time.substring(11, 16)}-${b.end_time.substring(11, 16)}`;
        console.log(
            `  [${i + 1}] ${timeRange}: ${b.count} (${lane}, sw=${b.slide_window_width})`,
        );
        total += b.count;
    });
    return total;
}

export function setup() {
    const runID = generateRunID();

    logHeader(`Throttle Load Test - ${config.scenarioName}`);
    console.log(`Run ID: ${runID}`);
    console.log(`Scenario: ${config.scenarioName}`);
    console.log(`Description: ${config.scenarioDescription}`);
    console.log(`Throttle URL: ${config.throttleUrl}`);
    console.log(`Stub URL: ${config.stubUrl}`);
    console.log(
        `Virtual time: ${config.virtualTimeStart} to ${config.virtualTimeEnd}`,
    );
    console.log(`Iterations: ${config.iterations}`);
    console.log(`Confirm window: ${config.confirmWindowMinutes} minutes`);
    console.log(`Buckets: ${config.seedBuckets.length}`);

    const totalCount = logBuckets(config.seedBuckets);
    console.log(`Total notifications: ${totalCount}`);
    console.log(`${"=".repeat(50)}\n`);

    // Reset test data
    const resetRes = http.post(`${config.stubUrl}/test/reset?run_id=${runID}`);
    if (!check(resetRes, { "reset successful": (r) => r.status === 200 })) {
        console.error(`Reset failed: ${resetRes.status} - ${resetRes.body}`);
        return null;
    }

    // Seed test data
    const seedRes = http.post(
        `${config.stubUrl}/test/seed?run_id=${runID}`,
        JSON.stringify({ buckets: config.seedBuckets }),
        { headers: { "Content-Type": "application/json" } },
    );
    if (!check(seedRes, { "seed successful": (r) => r.status === 200 })) {
        console.error(`Seed failed: ${seedRes.status} - ${seedRes.body}`);
        return null;
    }

    console.log(
        `Seeded ${config.seedBuckets.length} buckets with ${totalCount} total notifications\n`,
    );

    return {
        startTime: config.virtualTimeStart,
        endTime: config.virtualTimeEnd,
        runID,
        scenarioName: config.scenarioName,
    };
}

export default function (data) {
    if (!data) {
        console.error("Setup failed, skipping test");
        return;
    }

    const iteration = __ITER;
    const virtualStart = parseDate(data.startTime);
    const virtualEnd = parseDate(data.endTime);
    const windowMinutes = config.confirmWindowMinutes;

    const virtualFrom = addMinutes(virtualStart, iteration * windowMinutes);
    const virtualTo = addMinutes(virtualFrom, windowMinutes);

    if (virtualFrom >= virtualEnd) {
        console.log(`Iteration ${iteration}: Past virtual end time, skipping`);
        return;
    }

    const url = `${config.throttleUrl}/api/v1/throttle/batch?from=${formatRFC3339(virtualFrom)}&to=${formatRFC3339(virtualTo)}`;
    const startTime = Date.now();

    const res = http.post(url, null, {
        headers: {
            "Content-Type": "application/json",
            "X-Run-ID": data.runID,
        },
        timeout: "60s",
    });

    processingDuration.add(Date.now() - startTime);

    const success = check(res, {
        "status is 200": (r) => r.status === 200,
        "has valid response": (r) => {
            try {
                const body = JSON.parse(r.body);
                return (
                    body.processedCount !== undefined ||
                    body.processed_count !== undefined
                );
            } catch {
                return false;
            }
        },
    });

    if (success && res.status === 200) {
        try {
            const report = JSON.parse(res.body);
            const processed =
                report.processedCount || report.processed_count || 0;
            const succeeded = report.successCount || report.success_count || 0;
            const failed = report.failedCount || report.failed_count || 0;

            packetsProcessed.add(processed);
            packetsSuccess.add(succeeded);
            packetsFailed.add(failed);

            const timeWindow = `${virtualFrom.toISOString().substring(11, 16)}-${virtualTo.toISOString().substring(11, 16)}`;
            console.log(
                `[${timeWindow}] processed=${processed}, success=${succeeded}, failed=${failed}`,
            );
        } catch (e) {
            console.error(`Failed to parse response: ${e}`);
        }
    } else {
        console.error(
            `Iteration ${iteration} failed: ${res.status} - ${res.body}`,
        );
    }

    sleep(config.iterationDelay);
}

export function teardown(data) {
    if (!data) return;

    logHeader(`Load Test Complete - ${data.scenarioName}`);
    console.log(`Run ID: ${data.runID}`);

    if (config.resetOnTeardown) {
        http.post(`${config.stubUrl}/test/reset?run_id=${data.runID}`);
        console.log("Test data cleared");
    }

    console.log(`${"=".repeat(50)}\n`);
}
