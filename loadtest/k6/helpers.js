export function formatRFC3339(date) {
    return date.toISOString();
}

export function parseDate(dateStr) {
    return new Date(dateStr);
}

export function addMinutes(date, minutes) {
    return new Date(date.getTime() + minutes * 60 * 1000);
}

export function addMinutesToISO(isoTime, minutes) {
    const date = new Date(isoTime);
    date.setTime(date.getTime() + minutes * 60 * 1000);
    return date.toISOString().replace(".000Z", "Z");
}

export function generateRunID() {
    const now = new Date();
    const pad = (n) => String(n).padStart(2, "0");
    const year = now.getFullYear();
    const month = pad(now.getMonth() + 1);
    const day = pad(now.getDate());
    const hours = pad(now.getHours());
    const minutes = pad(now.getMinutes());
    const seconds = pad(now.getSeconds());
    return `${year}${month}${day}-${hours}${minutes}${seconds}`;
}
