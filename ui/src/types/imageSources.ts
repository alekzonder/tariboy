export type {
  ImageSource,
  ImageSourceBuildResult,
  ImageSourceValidateResult,
} from "@/lib/api-types.generated";

export interface CreateImageSource {
  name: string;
  prompt?: string;
}
