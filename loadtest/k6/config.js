import {
    buildRandomScenario,
    CONFIRM_WINDOW_MINUTES,
    getScenario,
    scenarioNames,
    scenarios,
} from "./scenarios.js";

const scenarioName = __ENV.SCENARIO || "warmup";

function parseEnvInt(key, defaultValue) {
    const val = __ENV[key];
    return val ? parseInt(val, 10) : defaultValue;
}

function parseEnvFloat(key, defaultValue) {
    const val = __ENV[key];
    return val !== undefined && val !== "" ? parseFloat(val) : defaultValue;
}

function buildConfig() {
    const baseConfig = {
        throttleUrl: __ENV.THROTTLE_URL || "http://localhost:8080",
        stubUrl: __ENV.STUB_URL || "http://localhost:8081",
        confirmWindowMinutes: CONFIRM_WINDOW_MINUTES,
        maxDuration: __ENV.MAX_DURATION || "30m",
        iterationDelay: parseEnvFloat("ITERATION_DELAY", 0),
        resetOnTeardown: __ENV.RESET_ON_TEARDOWN === "true",
    };

    let scenario;
    if (scenarioName === "random") {
        scenario = buildRandomScenario({
            baseTimeISO: __ENV.VIRTUAL_TIME_START || "2026-01-01T15:00:00Z",
            seed: __ENV.RANDOM_SEED || "",
            minBuckets: parseEnvInt("RANDOM_MIN_BUCKETS", 6),
            maxBuckets: parseEnvInt("RANDOM_MAX_BUCKETS", 15),
            minCount: parseEnvInt("RANDOM_MIN_COUNT", 50),
            maxCount: parseEnvInt("RANDOM_MAX_COUNT", 500),
            minDurationMinutes: parseEnvInt("RANDOM_MIN_DURATION_MINUTES", 1),
            maxDurationMinutes: parseEnvInt("RANDOM_MAX_DURATION_MINUTES", 5),
        });
    } else {
        scenario = getScenario(scenarioName);
        if (!scenario) {
            throw new Error(
                `Unknown scenario: ${scenarioName}. Available: ${scenarioNames.join(", ")}`,
            );
        }
    }

    return {
        ...baseConfig,
        scenarioName: scenario.name,
        scenarioDescription: scenario.description,
        virtualTimeStart: scenario.virtualTimeStart,
        virtualTimeEnd: scenario.virtualTimeEnd,
        iterations: scenario.iterations,
        seedBuckets: scenario.buckets,
    };
}

export const config = buildConfig();
export { scenarios, scenarioName };
