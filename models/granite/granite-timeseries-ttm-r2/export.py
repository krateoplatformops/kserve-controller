import torch
from tsfm_public.toolkit.get_model import get_model

# --- Configuration ---
TTM_MODEL_PATH = "ibm-granite/granite-timeseries-ttm-r2"
CONTEXT_LENGTH = 512
PREDICTION_LENGTH = 96

class TTMTracerWrapper(torch.nn.Module):
    def __init__(self, base_model):
        super().__init__()
        self.base_model = base_model

    def forward(self, past_values):
        # The base TTM model returns a TTMTSForecastingModelOutput object
        # which contains 'prediction_outputs' (the forecast)
        outputs = self.base_model(past_values=past_values)
        
        # Extract only the forecast tensor
        # Shape: [batch, prediction_length, channels]
        return outputs.prediction_outputs

def export_for_triton():
    # 1. Load the original model
    print("Loading original TTM model...")
    base_model = get_model(
        TTM_MODEL_PATH,
        context_length=CONTEXT_LENGTH,
        prediction_length=PREDICTION_LENGTH,
    )
    base_model.eval()

    # 2. Wrap the model
    wrapped_model = TTMTracerWrapper(base_model)
    wrapped_model.eval()

    # 3. Create dummy input for tracing
    # Triton expects [Batch, 512, 1] based on your config
    dummy_input = torch.randn(1, CONTEXT_LENGTH, 1)

    # 4. Trace the wrapped model
    print("Tracing wrapped model...")
    with torch.no_grad():
        traced_model = torch.jit.trace(wrapped_model, dummy_input)

    # 5. Save for Triton
    traced_model.save("model.pt")
    print("Export complete. Use this model.pt with your Triton config.")

if __name__ == "__main__":
    export_for_triton()