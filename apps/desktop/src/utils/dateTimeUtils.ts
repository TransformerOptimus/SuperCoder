import dayjs from "dayjs";
import utc from "dayjs/plugin/utc";

dayjs.extend(utc);

export const formatTypes: Record<string, string> = {
  centered: "MMM D, YYYY · h:mma",
  timestamp: "MMM D, YYYY [at] h:mm A",
  dateOnly: "MMM D, YYYY",
  timeOnly: "h:mm A",
  iso: "YYYY-MM-DDTHH:mm:ss.SSS[Z]",
  monthDay: "MMM D",
  time24Hour: "HH:mm",
  time12Hour: "h:mma",
  signalDate: "DD-MM-YYYY",
  formDateTime: "hh:mmA, MM/DD/YYYY",
  formDate: "MM/DD/YYYY",
  tableDateTime: "D MMM YYYY, h:mmA",
};

export const formatTimestamp = (
  dateString: string | null | undefined,
  formatType: string = "timestamp",
  customFormat: string | null = null,
): string => {
  if (!dateString) return formatType === "timestamp" ? "N/A" : "";

  try {
    // Parse as UTC (backend sends UTC timestamps), then convert to local
    const date = dayjs.utc(dateString);
    if (!date.isValid()) return formatType === "timestamp" ? "N/A" : "";

    let format: string;
    if (customFormat) {
      format = customFormat;
    } else if (formatTypes[formatType]) {
      format = formatTypes[formatType];
    } else {
      format = formatType;
    }

    return date.local().format(format);
  } catch (error) {
    if (formatType === "timestamp") {
      console.error("Error formatting timestamp:", error);
    }
    return formatType === "timestamp" ? "N/A" : "";
  }
};
