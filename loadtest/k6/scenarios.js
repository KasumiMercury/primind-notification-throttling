import {
    BASE_DATE,
    CONFIRM_WINDOW_MINUTES,
    LOOSE_SLIDE_WINDOW_SECONDS,
    SCENARIO_MINUTES,
    STRICT_SLIDE_WINDOW_SECONDS,
} from "./constants.js";
import { addMinutesToISO } from "./helpers.js";

export { CONFIRM_WINDOW_MINUTES };

// Timeline helpers

function toISO(time) {
    return `${BASE_DATE}T${time}:00Z`;
}

function buildEmptyTimeline() {
    return Array.from({ length: SCENARIO_MINUTES }, () => ({
        strict: 0,
        loose: 0,
    }));
}

function addRange(
    timeline,
    startMinute,
    endMinute,
    strictPerMinute,
    loosePerMinute,
) {
    if (
        startMinute < 0 ||
        endMinute > timeline.length ||
        startMinute >= endMinute
    ) {
        throw new Error(`invalid range: ${startMinute}-${endMinute}`);
    }
    for (let i = startMinute; i < endMinute; i++) {
        timeline[i].strict += strictPerMinute;
        timeline[i].loose += loosePerMinute;
    }
}

function timelineToBuckets(startISO, timeline) {
    const buckets = [];
    for (let i = 0; i < timeline.length; i++) {
        const minuteStart = addMinutesToISO(startISO, i);
        const minuteEnd = addMinutesToISO(startISO, i + 1);

        if (timeline[i].strict > 0) {
            buckets.push({
                start_time: minuteStart,
                end_time: minuteEnd,
                count: timeline[i].strict,
                slide_window_width: STRICT_SLIDE_WINDOW_SECONDS,
                task_type: "reminder",
            });
        }

        if (timeline[i].loose > 0) {
            buckets.push({
                start_time: minuteStart,
                end_time: minuteEnd,
                count: timeline[i].loose,
                slide_window_width: LOOSE_SLIDE_WINDOW_SECONDS,
                task_type: "notification",
            });
        }
    }
    return buckets;
}

function buildScenario({ name, description, startTime, buildTimeline }) {
    const virtualTimeStart = toISO(startTime);
    const timeline = buildEmptyTimeline();
    buildTimeline(timeline);

    return {
        name,
        description,
        virtualTimeStart,
        virtualTimeEnd: addMinutesToISO(virtualTimeStart, SCENARIO_MINUTES),
        iterations: SCENARIO_MINUTES / CONFIRM_WINDOW_MINUTES,
        buckets: timelineToBuckets(virtualTimeStart, timeline),
    };
}

export const scenarios = {
    warmup: buildScenario({
        name: "warmup",
        description: "2-hour warmup with periodic small bursts in both lanes",
        startTime: "10:00",
        buildTimeline: (timeline) => {
            for (const offset of [0, 30, 60, 90]) {
                addRange(timeline, offset, offset + 5, 3, 3);
            }
        },
    }),

    loose_burst: buildScenario({
        name: "loose_burst",
        description: "2-hour test with 3 large loose lane bursts",
        startTime: "11:00",
        buildTimeline: (timeline) => {
            for (const offset of [0, 40, 80]) {
                addRange(timeline, offset, offset + 5, 0, 80);
            }
        },
    }),

    strict_with_loose: buildScenario({
        name: "strict_with_loose",
        description: "2-hour test with strict bursts wrapped in loose traffic",
        startTime: "12:00",
        buildTimeline: (timeline) => {
            const applyCluster = (offset, strictPerMinute, loosePerMinute) => {
                addRange(timeline, offset, offset + 5, 0, loosePerMinute);
                addRange(timeline, offset + 5, offset + 10, strictPerMinute, 0);
                addRange(timeline, offset + 10, offset + 15, 0, loosePerMinute);
            };
            applyCluster(0, 60, 16);
            applyCluster(45, 60, 16);
            addRange(timeline, 90, 93, 0, 25);
            addRange(timeline, 93, 99, 80, 0);
            addRange(timeline, 99, 102, 0, 25);
        },
    }),

    strict_isolated: buildScenario({
        name: "strict_isolated",
        description:
            "2-hour test with isolated strict bursts and minimal loose",
        startTime: "13:00",
        buildTimeline: (timeline) => {
            const applyCluster = (offset, strictPerMinute) => {
                addRange(timeline, offset, offset + 2, 0, 2);
                addRange(timeline, offset + 2, offset + 8, strictPerMinute, 0);
                addRange(timeline, offset + 8, offset + 10, 0, 2);
            };
            applyCluster(0, 70);
            applyCluster(45, 70);
            addRange(timeline, 90, 92, 0, 1);
            addRange(timeline, 92, 100, 90, 0);
            addRange(timeline, 100, 102, 0, 1);
        },
    }),

    both_spike: buildScenario({
        name: "both_spike",
        description: "2-hour test with 3 simultaneous spikes from both lanes",
        startTime: "14:00",
        buildTimeline: (timeline) => {
            const spikes = [
                { offset: 10, strict: 60, loose: 60 },
                { offset: 50, strict: 60, loose: 60 },
                { offset: 90, strict: 80, loose: 80 },
            ];
            for (const spike of spikes) {
                addRange(
                    timeline,
                    spike.offset,
                    spike.offset + 5,
                    spike.strict,
                    spike.loose,
                );
            }
        },
    }),
};

// Random scenario generator

function createRng(seed) {
    if (!seed) {
        return Math.random;
    }
    let hash = 0;
    for (let i = 0; i < seed.length; i++) {
        hash = (hash * 31 + seed.charCodeAt(i)) >>> 0;
    }
    let state = hash >>> 0;
    return () => {
        state = (state * 1664525 + 1013904223) >>> 0;
        return state / 0x100000000;
    };
}

function randInt(rng, min, max) {
    return min > max ? min : min + Math.floor(rng() * (max - min + 1));
}

function addBucketToTimeline(timeline, startMinute, duration, count, lane) {
    if (duration <= 0 || count <= 0) return;

    const base = Math.floor(count / duration);
    const remainder = count % duration;

    for (let i = 0; i < duration; i++) {
        const idx = startMinute + i;
        if (idx < 0 || idx >= timeline.length) continue;

        const increment = base + (i < remainder ? 1 : 0);
        timeline[idx][lane] += increment;
    }
}

export function buildRandomScenario(options = {}) {
    const {
        name = "random",
        description = "2-hour random load pattern",
        baseTimeISO = `${BASE_DATE}T15:00:00Z`,
        seed = "",
        minBuckets = 6,
        maxBuckets = 15,
        minCount = 50,
        maxCount = 500,
        minDurationMinutes = 1,
        maxDurationMinutes = 5,
    } = options;

    const rng = createRng(seed);
    const timeline = buildEmptyTimeline();
    const bucketCount = randInt(rng, minBuckets, maxBuckets);

    for (let i = 0; i < bucketCount; i++) {
        const lane = rng() < 0.5 ? "strict" : "loose";
        const duration = randInt(rng, minDurationMinutes, maxDurationMinutes);
        const startMinute = randInt(
            rng,
            0,
            Math.max(0, SCENARIO_MINUTES - duration),
        );
        const count = randInt(rng, minCount, maxCount);
        addBucketToTimeline(timeline, startMinute, duration, count, lane);
    }

    return {
        name,
        description,
        virtualTimeStart: baseTimeISO,
        virtualTimeEnd: addMinutesToISO(baseTimeISO, SCENARIO_MINUTES),
        iterations: SCENARIO_MINUTES / CONFIRM_WINDOW_MINUTES,
        buckets: timelineToBuckets(baseTimeISO, timeline),
    };
}

// Exports

export const scenarioNames = [...Object.keys(scenarios), "random"];

export function getScenario(name) {
    return scenarios[name] || null;
}
